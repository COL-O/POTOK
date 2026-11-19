package stratos

import (
	"os"
	"path/filepath"
	"testing"

	"organism/modules/bus"
	"organism/modules/growth"
)

func TestStratosRecallToxicAndWALIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	walPath := filepath.Join(tmpDir, "stratos_test.wal")
	initialSwans := []growth.BlackSwanCase{
		{CaseID: 1, TestInput: 5.0, MinSafeOut: 0.0, MaxSafeOut: 0.8, CriticalTTS: 30},
	}
	store, err := NewStore(walPath, initialSwans)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	var pattern1 bus.HDCSlot
	pattern1.Words[0] = 0xAAAAAAAAAAAAAAAA
	store.RegisterPrecedent(HDCRecord{
		RecordID:        100,
		Pattern:         pattern1,
		CauseCode:       77,
		AntibodyAction:  888,
		RecommendedMask: 0x0F,
		Confidence:      0.95,
	})
	prec, found := store.RecallByHDC(&pattern1, 0.85)
	if !found || prec.PrecedentID != 100 || prec.AntibodyAction != 888 {
		t.Fatalf("expected precedent 100 to be recalled, got found=%v, prec=%+v", found, prec)
	}
	if store.RecallHits.Load() != 1 {
		t.Fatalf("expected RecallHits = 1, got %d", store.RecallHits.Load())
	}
	store.MarkLawToxic(100, 6999)
	if _, foundAfterToxic := store.RecallByHDC(&pattern1, 0.85); foundAfterToxic {
		t.Fatalf("toxic precedent 100 must NOT be recalled!")
	}
	if store.ToxicMarked.Load() != 1 {
		t.Fatalf("expected ToxicMarked = 1, got %d", store.ToxicMarked.Load())
	}
	store.RegisterBlackSwan(growth.BlackSwanCase{CaseID: 2, TestInput: 12.0, MinSafeOut: 0.0, MaxSafeOut: 0.5, CriticalTTS: 15})
	swans := store.FetchBlackSwans(1)
	if len(swans) != 1 || swans[0].CaseID != 2 {
		t.Fatalf("expected latest black swan CaseID=2, got %+v", swans)
	}
	eq := growth.SINDyEquation{C0: 0.1, C1: 0.25, FitError: 0.01, EquationID: 500}
	lut := growth.CompiledLUT{MinVal: 0, MaxVal: 10, Knots: []float32{0.1, 0.5, 0.9}, EquationID: 500}
	store.RecordNewLaw(eq, lut, 500)
	if store.LawsRecorded.Load() != 1 {
		t.Fatalf("expected LawsRecorded = 1, got %d", store.LawsRecorded.Load())
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close failed: %v", err)
	}
	rawWAL, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatalf("ReadFile(walPath) failed: %v", err)
	}
	const frameSize = 56
	if len(rawWAL) != 4*frameSize {
		t.Fatalf("WAL size = %d bytes, want %d bytes (4 frames)", len(rawWAL), 4*frameSize)
	}
	expectedTypes := []byte{0x03, 0x02, 0x04, 0x01}
	for i, wantType := range expectedTypes {
		frame := rawWAL[i*frameSize : (i+1)*frameSize]
		if frame[0] != wantType {
			t.Fatalf("frame %d type = %#x, want %#x", i, frame[0], wantType)
		}
		var nonZero bool
		for _, b := range frame[24:56] {
			if b != 0 {
				nonZero = true
				break
			}
		}
		if !nonZero {
			t.Fatalf("frame %d has empty SHA-256 seal!", i)
		}
	}
}
