// Package seed — генерация синтетического мира организма (blueprint):
// заводы, изделия, BOM-связи, рабочие центры. Всё детерминировано по Seed:
// один и тот же конфиг — один и тот же мир, чтобы тесты и запуски совпадали.
package seed

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/COL-O/POTOK/contracts"
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

	// Изделия по слоям. NumRaw должен делиться на число категорий сырья (6).
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

// DefaultConfig — конфиг по умолчанию: 100 заводов, ~480 изделий.
func DefaultConfig() Config {
	return Config{
		NumQuarries:   8,
		NumShops:      32,
		NumAssembly:   20,
		NumHospitals:  8,
		NumRecyclers:  12,
		NumWarehouses: 20,

		NumRaw:         60,
		NumUnits:       240,
		NumSubassembly: 120,
		NumProducts:    60,

		MinChildren: 2,
		MaxChildren: 5,

		Seed: 42,
	}
}

// interner — выдаёт стабильные числовые ID по строковым кодам.
// Повторный Intern того же кода возвращает тот же ID (нужно дублёрам).
type interner struct {
	mu        sync.Mutex
	factories map[string]uint16
	products  map[string]uint32
	nextFact  uint16
	nextProd  uint32
}

func newInterner() *interner {
	return &interner{
		factories: make(map[string]uint16),
		products:  make(map[string]uint32),
		nextFact:  1,
		nextProd:  1,
	}
}

// InternFactory — ID завода по коду (создаёт при первом обращении).
func (in *interner) InternFactory(code string) (uint16, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if id, ok := in.factories[code]; ok {
		return id, nil
	}
	id := in.nextFact
	in.nextFact++
	in.factories[code] = id
	return id, nil
}

// InternProduct — ID изделия по SKU (создаёт при первом обращении).
func (in *interner) InternProduct(sku string) (uint32, error) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if id, ok := in.products[sku]; ok {
		return id, nil
	}
	id := in.nextProd
	in.nextProd++
	in.products[sku] = id
	return id, nil
}

// Generator — генератор мира. Порядок генерации важен:
// заводы → изделия → BOM → BIM (BOM и BIM опираются на предыдущие слои).
type Generator struct {
	cfg    Config
	rng    *rand.Rand
	intern *interner

	factories   []contracts.Factory
	products    []contracts.Product
	bomEdges    []contracts.BOMEdge
	workCenters []contracts.WorkCenter
	connections []contracts.Connection

	parentToChildren map[uint32][]uint32
}

// NewGenerator — генератор с конфигом.
func NewGenerator(cfg Config) *Generator {
	return &Generator{
		cfg:              cfg,
		rng:              rand.New(rand.NewSource(cfg.Seed)),
		intern:           newInterner(),
		parentToChildren: make(map[uint32][]uint32),
	}
}

// Generate — сгенерировать весь мир.
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
	if err := g.generateBIM(); err != nil {
		return fmt.Errorf("seed bim: %w", err)
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
