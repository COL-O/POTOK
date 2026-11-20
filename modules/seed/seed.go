// Package seed — генерация синтетического мира организма (blueprint):
// заводы, изделия, BOM-связи, рабочие центры. Всё детерминировано по Seed:
// один и тот же конфиг — один и тот же мир, чтобы тесты и запуски совпадали.
//
// Порядок генерации — по правилу «сначала атомы, потом молекулы»:
// заводы → изделия → BOM (молекулы) → валидация DAG → BIM.
package seed

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"organism/contracts"
	"organism/core"
)

// Config — параметры генерации мира.
type Config struct {
	// Заводы по ролям. В сумме по умолчанию — 100 заводов общего поля.
	NumQuarries   int
	NumShops      int
	NumAssembly   int
	NumHospitals  int
	NumRecyclers  int
	NumWarehouses int

	// Изделия по слоям (в сумме по умолчанию — 10 000+).
	// NumRaw должен делиться на число категорий сырья (6).
	NumRaw         int
	NumUnits       int
	NumSubassembly int
	NumProducts    int

	// BOM: детей у одного родителя — от MinChildren до MaxChildren.
	MinChildren int
	MaxChildren int

	// Seed — зерно детерминированного генератора.
	Seed int64
}

// DefaultConfig — конфиг по умолчанию: 100 заводов, 10 000 изделий.
func DefaultConfig() Config {
	return Config{
		NumQuarries:   8,
		NumShops:      32,
		NumAssembly:   20,
		NumHospitals:  8,
		NumRecyclers:  12,
		NumWarehouses: 20,

		NumRaw:         600,
		NumUnits:       4000,
		NumSubassembly: 3000,
		NumProducts:    2400,

		MinChildren: 2,
		MaxChildren: 5,

		Seed: 42,
	}
}

// Generator — генератор мира. Порядок генерации важен:
// заводы → изделия → BOM → BIM (BOM и BIM опираются на предыдущие слои).
type Generator struct {
	cfg    Config
	rng    *rand.Rand
	intern *contracts.InternDictionary

	factories   []contracts.Factory
	products    []contracts.Product
	bomEdges    []contracts.BOMEdge
	workCenters []contracts.WorkCenter
	connections []contracts.Connection

	parentToChildren map[uint32][]uint32
	generated        bool

	// createdAt — единая метка времени мира: мир генерируется одним актом,
	// и все сущности должны иметь created_at (иначе Meta.Validate отвергает их).
	createdAt time.Time
}

// NewGenerator — генератор с конфигом.
func NewGenerator(cfg Config) *Generator {
	return &Generator{
		cfg:              cfg,
		rng:              rand.New(rand.NewSource(cfg.Seed)),
		intern:           contracts.NewInternDictionary(),
		parentToChildren: make(map[uint32][]uint32),
		createdAt:        time.Now().UTC(),
	}
}

// Intern — словарь код → ID, которым пользуется генератор
// (дублёры и повторные коды получают тот же ID).
func (g *Generator) Intern() *contracts.InternDictionary { return g.intern }

// Generate — сгенерировать весь мир и проверить BOM-граф.
func (g *Generator) Generate() error {
	if g.cfg.MinChildren < 1 || g.cfg.MaxChildren < g.cfg.MinChildren {
		return fmt.Errorf("seed: invalid children range %d..%d", g.cfg.MinChildren, g.cfg.MaxChildren)
	}
	if err := g.generateFactories(); err != nil {
		return fmt.Errorf("seed factories: %w", err)
	}
	if err := g.generateProducts(); err != nil {
		return fmt.Errorf("seed products: %w", err)
	}
	if err := g.generateBOM(); err != nil {
		return fmt.Errorf("seed bom: %w", err)
	}
	if err := g.validateDAG(); err != nil {
		return fmt.Errorf("seed validate DAG: %w", err)
	}
	if err := g.generateBIM(); err != nil {
		return fmt.Errorf("seed bim: %w", err)
	}
	g.generated = true
	return nil
}

// validateDAG — 5 проверок BOM-графа после генерации:
//  1. все связи ссылаются на известные изделия;
//  2. нет самосвязей (parent == child);
//  3. связи только между соседними слоями;
//  4. все связи привязаны к существующим заводам;
//  5. граф ацикличен (топологическая сортировка проходит целиком).
func (g *Generator) validateDAG() error {
	layerOf := make(map[uint32]contracts.ProductLayer, len(g.products))
	for _, p := range g.products {
		layerOf[p.ID] = p.Layer
	}
	factoryKnown := make(map[uint16]bool, len(g.factories))
	for _, f := range g.factories {
		factoryKnown[f.ID] = true
	}

	children := make(map[uint32][]uint32, len(g.products))
	indeg := make(map[uint32]int, len(g.products))
	for _, p := range g.products {
		indeg[p.ID] = 0
	}

	for _, e := range g.bomEdges {
		parentLayer, ok := layerOf[e.ParentID]
		if !ok { // проверка 1
			return fmt.Errorf("%w: parent %d", contracts.ErrUnknownProduct, e.ParentID)
		}
		childLayer, ok := layerOf[e.ChildID]
		if !ok { // проверка 1
			return fmt.Errorf("%w: child %d", contracts.ErrUnknownProduct, e.ChildID)
		}
		if e.ParentID == e.ChildID { // проверка 2
			return fmt.Errorf("%w: self edge on product %d", contracts.ErrBOMCycle, e.ParentID)
		}
		if !contracts.IsAdjacentLayer(parentLayer, childLayer) { // проверка 3
			return fmt.Errorf("%w: %d (layer %d) -> %d (layer %d)",
				contracts.ErrNonAdjacentLayer, e.ParentID, parentLayer, e.ChildID, childLayer)
		}
		if !factoryKnown[e.FactoryID] { // проверка 4
			return fmt.Errorf("seed: BOM edge %d->%d: unknown factory %d", e.ParentID, e.ChildID, e.FactoryID)
		}
		children[e.ParentID] = append(children[e.ParentID], e.ChildID)
		indeg[e.ChildID]++
	}

	// Проверка 5: Kahn — если отсортировались не все вершины, есть цикл.
	queue := make([]uint32, 0, len(indeg))
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	sorted := 0
	for len(queue) > 0 {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		sorted++
		for _, c := range children[id] {
			indeg[c]--
			if indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if sorted != len(g.products) {
		return fmt.Errorf("%w: sorted %d of %d", contracts.ErrBOMCycle, sorted, len(g.products))
	}
	return nil
}

// Factories — сгенерированные заводы (копия среза).
func (g *Generator) Factories() []contracts.Factory {
	return append([]contracts.Factory(nil), g.factories...)
}

// Products — сгенерированные изделия (копия среза).
func (g *Generator) Products() []contracts.Product {
	return append([]contracts.Product(nil), g.products...)
}

// BOMEdges — сгенерированные BOM-связи (копия среза).
func (g *Generator) BOMEdges() []contracts.BOMEdge {
	return append([]contracts.BOMEdge(nil), g.bomEdges...)
}

// WorkCenters — сгенерированные рабочие центры (копия среза).
func (g *Generator) WorkCenters() []contracts.WorkCenter {
	return append([]contracts.WorkCenter(nil), g.workCenters...)
}

// Connections — сгенерированные связи рабочих центров (копия среза).
func (g *Generator) Connections() []contracts.Connection {
	return append([]contracts.Connection(nil), g.connections...)
}

// Generated — мир уже сгенерирован.
func (g *Generator) Generated() bool { return g.generated }

// ---- seed как модуль организма ----

// Module — seed в виде модуля реестра: мир генерируется на Start.
// В текущей сборке main вызывает Generator напрямую до реестра;
// Module — для сборки, где seed живёт внутри организма.
type Module struct {
	gen *Generator
}

// NewModule — модуль seed поверх генератора.
func NewModule(gen *Generator) *Module { return &Module{gen: gen} }

// Name — имя модуля.
func (m *Module) Name() string { return "seed" }

// Init — окружение seed не нужно: он сам создаёт мир.
func (m *Module) Init(ctx context.Context, env *core.Env) error { return nil }

// Start — сгенерировать мир, если ещё не сгенерирован.
func (m *Module) Start(ctx context.Context) error {
	if m.gen.Generated() {
		return nil
	}
	return m.gen.Generate()
}

// Stop — останавливать нечего.
func (m *Module) Stop(ctx context.Context) error { return nil }

// Generator — генератор модуля.
func (m *Module) Generator() *Generator { return m.gen }
