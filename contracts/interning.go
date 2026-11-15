package contracts

import "sync"

// InternDictionary — словарь «строковый код → числовой ID» организма.
// Заводы и изделия внутри общего поля живут под короткими числовыми ID,
// а человекочитаемые коды (QUARRY_01, steel_001) интернируются один раз.
// Повторное интернирование того же кода возвращает тот же ID —
// на этом держатся дублёры заводов в seed.
type InternDictionary struct {
	mu          sync.Mutex
	factories   map[string]uint16
	products    map[string]uint32
	nextFactory uint16
	nextProduct uint32
}

// NewInternDictionary — пустой словарь (ID выдаются с 1).
func NewInternDictionary() *InternDictionary {
	return &InternDictionary{
		factories:   make(map[string]uint16),
		products:    make(map[string]uint32),
		nextFactory: 1,
		nextProduct: 1,
	}
}

// InternFactory — ID завода по коду (создаёт при первом обращении).
func (d *InternDictionary) InternFactory(code string) (uint16, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.factories[code]; ok {
		return id, nil
	}
	id := d.nextFactory
	d.nextFactory++
	d.factories[code] = id
	return id, nil
}

// InternProduct — ID изделия по SKU (создаёт при первом обращении).
func (d *InternDictionary) InternProduct(sku string) (uint32, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.products[sku]; ok {
		return id, nil
	}
	id := d.nextProduct
	d.nextProduct++
	d.products[sku] = id
	return id, nil
}

// LoadFactories — пакетно занести коды заводов, вернуть ID в том же порядке.
func (d *InternDictionary) LoadFactories(codes []string) ([]uint16, error) {
	out := make([]uint16, 0, len(codes))
	for _, code := range codes {
		id, err := d.InternFactory(code)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// LoadProducts — пакетно занести SKU изделий, вернуть ID в том же порядке.
func (d *InternDictionary) LoadProducts(skus []string) ([]uint32, error) {
	out := make([]uint32, 0, len(skus))
	for _, sku := range skus {
		id, err := d.InternProduct(sku)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// Snapshot — копии словарей (код → ID) для сохранения/диагностики.
func (d *InternDictionary) Snapshot() (factories map[string]uint16, products map[string]uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	factories = make(map[string]uint16, len(d.factories))
	for k, v := range d.factories {
		factories[k] = v
	}
	products = make(map[string]uint32, len(d.products))
	for k, v := range d.products {
		products[k] = v
	}
	return factories, products
}
