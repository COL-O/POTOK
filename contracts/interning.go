package contracts

import (
	"fmt"
	"sync"
)

type InternDictionary struct {
	mu            sync.RWMutex
	factoryByCode map[string]uint16
	factoryByID   map[uint16]string
	nextFactoryID uint16
	productBySKU  map[string]uint32
	productByID   map[uint32]string
	nextProductID uint32
}

func NewInternDictionary() *InternDictionary {
	return &InternDictionary{
		factoryByCode: make(map[string]uint16),
		factoryByID:   make(map[uint16]string),
		productBySKU:  make(map[string]uint32),
		productByID:   make(map[uint32]string),
		nextFactoryID: 1,
		nextProductID: 1,
	}
}

func (d *InternDictionary) InternFactory(code string) (uint16, error) {
	if code == "" {
		return 0, fmt.Errorf("intern: пустой factory code")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.factoryByCode[code]; ok {
		return id, nil
	}
	if d.nextFactoryID == 0 {
		return 0, fmt.Errorf("intern: переполнение factory ID (uint16)")
	}
	id := d.nextFactoryID
	d.factoryByCode[code] = id
	d.factoryByID[id] = code
	d.nextFactoryID++
	return id, nil
}

func (d *InternDictionary) FactoryCode(id uint16) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	code, ok := d.factoryByID[id]
	return code, ok
}

func (d *InternDictionary) FactoryCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.factoryByCode)
}

func (d *InternDictionary) InternProduct(sku string) (uint32, error) {
	if sku == "" {
		return 0, fmt.Errorf("intern: пустой product sku")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id, ok := d.productBySKU[sku]; ok {
		return id, nil
	}
	if d.nextProductID == 0 {
		return 0, fmt.Errorf("intern: переполнение product ID (uint32)")
	}
	id := d.nextProductID
	d.productBySKU[sku] = id
	d.productByID[id] = sku
	d.nextProductID++
	return id, nil
}

func (d *InternDictionary) ProductSKU(id uint32) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	sku, ok := d.productByID[id]
	return sku, ok
}

func (d *InternDictionary) ProductCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.productBySKU)
}

func (d *InternDictionary) LoadFactories(factories []Factory) (int, error) {
	loaded := 0
	for _, f := range factories {
		if err := f.Validate(); err != nil {
			return loaded, fmt.Errorf("load factory %q: %w", f.Code, err)
		}
		d.mu.Lock()
		if f.ID != 0 {
			if existing, ok := d.factoryByID[f.ID]; ok && existing != f.Code {
				d.mu.Unlock()
				return loaded, fmt.Errorf("load factory %q: ID %d уже занят %q", f.Code, f.ID, existing)
			}
			d.factoryByCode[f.Code] = f.ID
			d.factoryByID[f.ID] = f.Code
			if f.ID >= d.nextFactoryID {
				d.nextFactoryID = f.ID + 1
			}
		} else {
			if _, ok := d.factoryByCode[f.Code]; !ok {
				id := d.nextFactoryID
				d.factoryByCode[f.Code] = id
				d.factoryByID[id] = f.Code
				d.nextFactoryID++
			}
		}
		d.mu.Unlock()
		loaded++
	}
	return loaded, nil
}

func (d *InternDictionary) LoadProducts(products []Product) (int, error) {
	loaded := 0
	for _, p := range products {
		if err := p.Validate(); err != nil {
			return loaded, fmt.Errorf("load product %q: %w", p.SKU, err)
		}
		d.mu.Lock()
		if p.ID != 0 {
			if existing, ok := d.productByID[p.ID]; ok && existing != p.SKU {
				d.mu.Unlock()
				return loaded, fmt.Errorf("load product %q: ID %d уже занят %q", p.SKU, p.ID, existing)
			}
			d.productBySKU[p.SKU] = p.ID
			d.productByID[p.ID] = p.SKU
			if p.ID >= d.nextProductID {
				d.nextProductID = p.ID + 1
			}
		} else {
			if _, ok := d.productBySKU[p.SKU]; !ok {
				id := d.nextProductID
				d.productBySKU[p.SKU] = id
				d.productByID[id] = p.SKU
				d.nextProductID++
			}
		}
		d.mu.Unlock()
		loaded++
	}
	return loaded, nil
}

type FactorySnapshot struct {
	Code string `json:"code"`
	ID   uint16 `json:"id"`
}

type ProductSnapshot struct {
	SKU string `json:"sku"`
	ID  uint32 `json:"id"`
}

func (d *InternDictionary) SnapshotFactories() []FactorySnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]FactorySnapshot, 0, len(d.factoryByCode))
	for code, id := range d.factoryByCode {
		out = append(out, FactorySnapshot{Code: code, ID: id})
	}
	return out
}

func (d *InternDictionary) SnapshotProducts() []ProductSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]ProductSnapshot, 0, len(d.productBySKU))
	for sku, id := range d.productBySKU {
		out = append(out, ProductSnapshot{SKU: sku, ID: id})
	}
	return out
}

func (d *InternDictionary) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.factoryByCode = make(map[string]uint16)
	d.factoryByID = make(map[uint16]string)
	d.productBySKU = make(map[string]uint32)
	d.productByID = make(map[uint32]string)
	d.nextFactoryID = 1
	d.nextProductID = 1
}
