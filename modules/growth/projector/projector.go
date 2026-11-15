// Package projector — обратный отсчёт growth: «что можно собрать
// из излишка». Идёт по обратному индексу BOM (ребёнок → родители)
// и предлагает can_build. Строго read-only: в основные данные не пишет.
//
// В авторском скелете ID подсказок давал google/uuid; здесь ID —
// случайный hex на crypto/rand (внешние зависимости недоступны).
package projector

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/COL-O/POTOK/contracts"
	"github.com/COL-O/POTOK/modules/bim"
	"github.com/COL-O/POTOK/modules/graph"
)

// Projector — обратный BOM-проход с ограничениями BIM.
type Projector struct {
	bom *graph.BOMGraph
	bim *bim.BIM
}

// New — projector поверх BOM-графа и BIM.
func New(b *graph.BOMGraph, bm *bim.BIM) *Projector {
	return &Projector{bom: b, bim: bm}
}

// Suggest — обратный отсчёт: что можно собрать из излишка.
// Read-only: не пишет в основные данные.
// Скелет: по одному предложению на каждую связь ребёнок→родитель,
// CanBuild и Confidence — заглушки до точных формул.
func (p *Projector) Suggest() []contracts.Suggestion {
	if p.bom == nil {
		return nil
	}
	out := []contracts.Suggestion{}
	// Для каждого родителя с детьми — предложить can_build.
	for childID, parents := range p.bom.Reverse() {
		for _, parentID := range parents {
			out = append(out, contracts.Suggestion{
				Meta:         contracts.Meta{Source: "projector", Branch: "main"},
				SuggestionID: newSuggestionID(),
				ProductID:    parentID,
				CanBuild:     1,
				Source:       contracts.SourceReverseBOM,
				Confidence:   0.5,
			})
		}
		_ = childID
	}
	return out
}

// BIM — доступ к ограничениям (capacity) для следующих версий Suggest.
func (p *Projector) BIM() *bim.BIM { return p.bim }

func newSuggestionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "suggestion-0"
	}
	return "suggestion-" + hex.EncodeToString(b[:])
}
