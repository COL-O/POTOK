package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/growth"
	"organism/modules/network"
	"organism/modules/polygon"
	"organism/modules/stratos"
)

type OrganismBridge struct {
	mu           sync.Mutex
	frontRouter  *front.Router
	hdcOrgan     *front.HDCOrgan
	tsetlinOrgan *front.TsetlinOrgan
	kanOrgan     *polygon.KANOrgan

	genCounter   atomic.Uint64
	kanVersion   atomic.Uint64
	frontVersion atomic.Uint64

	activeProtos    []front.HDCPrototype
	activeVetoRules []front.VetoRule
	quarantinePool  []front.HDCPrototype
	currentKAN      *polygon.KANRuleTable
	snapshots       map[uint64]organismGenomeState
}

type organismGenomeState struct {
	protos    []front.HDCPrototype
	vetoRules []front.VetoRule
	kanTable  *polygon.KANRuleTable
}

func NewOrganismBridge(fr *front.Router, hdc *front.HDCOrgan, ts *front.TsetlinOrgan, kan *polygon.KANOrgan, initProtos []front.HDCPrototype, initKAN *polygon.KANRuleTable) *OrganismBridge {
	b := &OrganismBridge{
		frontRouter:  fr,
		hdcOrgan:     hdc,
		tsetlinOrgan: ts,
		kanOrgan:     kan,
		activeProtos: append([]front.HDCPrototype(nil), initProtos...),
		currentKAN:   initKAN,
		snapshots:    make(map[uint64]organismGenomeState),
	}
	b.genCounter.Store(1)
	b.kanVersion.Store(initKAN.Version)
	b.frontVersion.Store(1)
	return b
}

func (b *OrganismBridge) ProposeQuarantineReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.quarantinePool = append(b.quarantinePool, proto)
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) ProposeFrontReflex(proto front.HDCPrototype, clause front.TsetlinClause) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeProtos = append(b.activeProtos, proto)
	v := b.frontVersion.Add(1)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{
		Version:    v,
		Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...),
	})
}

func (b *OrganismBridge) ApplyVetoRule(rule front.VetoRule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeVetoRules = append(b.activeVetoRules, rule)
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) RehabilitateVeto(mask uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var kept []front.VetoRule
	for _, r := range b.activeVetoRules {
		r.Mask &^= mask
		if r.Mask != 0 {
			kept = append(kept, r)
		}
	}
	b.activeVetoRules = kept
	v := b.frontVersion.Add(1)
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    v,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
}

func (b *OrganismBridge) DeployPolygonKAN(table *polygon.KANRuleTable, alpha float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentKAN = table
	b.kanVersion.Store(table.Version)
	b.kanOrgan.UpdateTableRCU(table)
}

func (b *OrganismBridge) SnapshotCurrentGenome() growth.GenomeSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	gen := b.genCounter.Load()
	b.snapshots[gen] = organismGenomeState{
		protos:    append([]front.HDCPrototype(nil), b.activeProtos...),
		vetoRules: append([]front.VetoRule(nil), b.activeVetoRules...),
		kanTable:  b.currentKAN,
	}
	return growth.GenomeSnapshot{
		GenerationID: gen,
		Timestamp:    time.Now().UnixNano(),
		KANVersion:   b.kanVersion.Load(),
		FrontVersion: b.frontVersion.Load(),
	}
}

func (b *OrganismBridge) RestoreGenomeSnapshot(snap growth.GenomeSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.snapshots[snap.GenerationID]
	if !ok {
		return
	}
	b.activeProtos = append([]front.HDCPrototype(nil), st.protos...)
	b.activeVetoRules = append([]front.VetoRule(nil), st.vetoRules...)
	b.currentKAN = st.kanTable
	vFront := b.frontVersion.Add(1)
	b.hdcOrgan.UpdateTableRCU(&front.HDCTable{
		Version:    vFront,
		Prototypes: append([]front.HDCPrototype(nil), b.activeProtos...),
	})
	b.frontRouter.UpdatePolicyRCU(&front.RouterPolicy{
		Version:    vFront,
		VetoRules:  append([]front.VetoRule(nil), b.activeVetoRules...),
		Quarantine: append([]front.HDCPrototype(nil), b.quarantinePool...),
	})
	if st.kanTable != nil {
		b.kanOrgan.UpdateTableRCU(st.kanTable)
	}
}

func main() {
	fmt.Println("=== ИНИЦИАЛИЗАЦИЯ ЦИФРОВОГО ОРГАНИЗМА (4 КОНТУРА ВРЕМЕНИ) ===")

	walPath := filepath.Join(os.TempDir(), "organism_stratos.wal")
	memoryStore, err := stratos.NewStore(walPath, []growth.BlackSwanCase{
		{CaseID: 1, TestInput: 5.0, MinSafeOut: 0.0, MaxSafeOut: 1.0, CriticalTTS: 30},
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации stratos: %v", err)
	}
	defer memoryStore.Close()

	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   4096,
		HDCRingSize:     4096,
		GraphArenaSize:  256 * 1024,
		TensorArenaSize: 512 * 1024,
		MetaRingSize:    4096,
		ErrorRingSize:   1024,
		VectorArenaSize: 128 * 1024,
		AudioArenaSize:  128 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации Шины: %v", err)
	}
	defer coreBus.Close()

	scratchPool, err := bus.NewScratchPool(64, 4096)
	if err != nil {
		log.Fatalf("Ошибка создания ScratchPool: %v", err)
	}
	defer scratchPool.Close()

	var routinePattern bus.HDCSlot
	initProtos := []front.HDCPrototype{
		{Pattern: routinePattern, ClassID: 1, BaseMask: front.BaseRoutineMask | front.BitTTL, MinSim: 0.85},
	}
	hdcOrgan := front.NewHDCOrgan(initProtos)
	tsetlinOrgan := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 100, Weight: 0.95},
	})

	registry := bus.NewOrganRegistry()
	_ = registry.HotSwap(hdcOrgan)
	_ = registry.HotSwap(tsetlinOrgan)
	_ = registry.HotSwap(&front.TTLOrgan{MinSafeTTS: 20})
	_ = registry.HotSwap(&front.CapacityOrgan{MaxEdgeLoad: 10.0})
	_ = registry.HotSwap(&front.DStarLiteOrgan{})
	_ = registry.HotSwap(&front.MCTSOrgan{MaxRollouts: 16})
	_ = registry.HotSwap(&front.PasportOrgan{})

	prefetcher := front.NewGraphPrefetcher(front.PrefetchConfig{
		DiskBandwidthMBps:   3000,
		WindowDuration:      10 * time.Millisecond,
		MaxTokensPerWindow:  4,
		CriticalTTSThresh:   25,
		MinPriorityOverride: 200,
	}, nil)
	defer prefetcher.Close()

	frontRouter := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 20 * time.Millisecond,
		MaxFlapsInWindow:  4,
		FlapWindowNs:      int64(500 * time.Millisecond),
		QuarantineTrials:  2,
	}, coreBus, registry, scratchPool, hdcOrgan, tsetlinOrgan, prefetcher)

	initKAN := &polygon.KANRuleTable{
		Version:   1,
		MinVal:    0.0,
		MaxVal:    10.0,
		Knots:     []float32{0.1, 0.5, 0.9},
		CauseCode: 77,
	}
	kanOrgan := polygon.NewKANOrgan(initKAN)
	polyTsetlin := polygon.NewPolygonTsetlinOrgan([]polygon.AntibodyRule{
		{CauseCode: 77, AntibodyAction: 888, ProposeMask: front.BaseRoutineMask | front.BitDStarLite, Confidence: 0.91},
	})

	bridge := NewOrganismBridge(frontRouter, hdcOrgan, tsetlinOrgan, kanOrgan, initProtos, initKAN)

	polygonRouter := polygon.NewRouter(
		polygon.RouterConfig{
			MinPrecedentSim:  0.85,
			RecurrenceLimit:  3,
			RecurrenceWindow: time.Minute,
			DefaultVetoTTL:   time.Minute,
		},
		coreBus, registry, scratchPool, memoryStore, bridge,
		&polygon.MatrixProfileOrgan{WindowSize: 4, DiscordThresh: 2.0},
		kanOrgan,
		&polygon.CfCOrgan{DecayTau: 2.0, HorizonStep: 1.0},
		&polygon.CBSOrgan{MaxNodeCapacity: 10.0},
		&polygon.ESNOrgan{LeakRate: 0.3},
		polyTsetlin,
	)

	growthRouter := growth.NewRouter(
		growth.RouterConfig{
			BlackSwanBatchSize: 5,
			InitialAlpha:       0.1,
			AlphaStep:          0.3,
			PanicSurpriseRate:  10,
		},
		coreBus, scratchPool, memoryStore, bridge,
		&growth.GraphSAGEOrgan{MaxDegreeChange: 2.0},
		&growth.ActiveInferenceOrgan{ExpectedBaseline: 1.0, SurpriseThresh: 0.5},
		&growth.SINDyOrgan{MaxFitError: 1.5},
		&growth.LUTKANOrgan{Resolution: 8, SafetyPad: 0.25},
		&growth.GrowthMCTSOrgan{Simulations: 16, Horizon: 3},
		&growth.TDMPC2Organ{BlendSteps: 4},
	)

	dashboard := NewDashboardServer(":8080", coreBus, registry, frontRouter, polygonRouter, growthRouter, memoryStore)
	if err := dashboard.Start(); err != nil {
		log.Fatalf("Ошибка запуска HTTP-дашборда :8080: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dashboard.Shutdown(ctx)
	}()
	fmt.Println("[Dashboard] HTTP-сервер прозрачности готов на :8080 (/health, /metrics, /prometheus)")

	gateway := network.NewSensoryGateway(coreBus, frontRouter)
	if err := gateway.StartUDP(":9000"); err != nil {
		log.Fatalf("Ошибка запуска UDP-шлюза :9000: %v", err)
	}
	defer func() {
		if err := gateway.Stop(); err != nil {
			log.Printf("[Shutdown] Ошибка остановки UDP-шлюза: %v", err)
		}
	}()
	fmt.Println("[Gateway]   UDP-рецептор телеметрии и графов запущен на :9000")

	id1 := coreBus.Publish(&routinePattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, _ := frontRouter.Step(id1)
	fmt.Printf("[Такт 1 | front]   Штатный рейс: Status=%d, Confidence=%.2f | UDP-пакетов=%d\n", v1.Status, v1.Confidence, gateway.PacketsReceived.Load())

	var anomalyHDC bus.HDCSlot
	for i := range anomalyHDC.Words {
		anomalyHDC.Words[i] = 0xF0F0F0F0F0F0F0F0
	}
	series1 := []float32{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 7.5, 8.0, 8.2, 8.5}
	tOff1, _ := coreBus.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series1)
	gOff1, _ := coreBus.Graphs.Write(0, []uint32{0, 1, 1}, []uint32{1}, []float32{2.0})
	id2 := coreBus.Publish(&anomalyHDC, gOff1, tOff1, 50, 150)
	v2, _ := frontRouter.Step(id2)
	fmt.Printf("[Такт 2 | front]   Слепое пятно HDC: Status=%d -> взведён Surprise!\n", v2.Status)

	polyVerdict, audit2, _ := polygonRouter.Diagnose(id2)
	fmt.Printf("[Такт 2 | polygon] Диагноз поставлен: Cause=%d, Antibody=%d, Confidence=%.2f\n", audit2.CauseCode, polyVerdict.ActionCode, polyVerdict.Confidence)

	// Аномалия законна, но не узнана HDC (как в полёте): dx=const=2.2.
	// Прежняя серия со скачком не была функцией dx=f(x) — growth браковал закон (6002).
	series2 := []float32{1.0, 3.2, 5.4, 7.6, 9.8, 12.0, 14.2, 16.4, 18.6, 20.8, 23.0, 25.2}
	tOff2, _ := coreBus.Tensors.Write4D(0, [4]uint16{1, 1, 1, 12}, series2)
	id3 := coreBus.Publish(&anomalyHDC, gOff1, tOff2, 40, 220)
	_, _, _ = polygonRouter.Diagnose(id3)
	fmt.Printf("[Такт 3 | polygon] Выход за паспорт KAN (%.1f > 10.0) -> Эскалация в growth!\n", series2[len(series2)-1])
	evoAudit, _ := growthRouter.Evolve(id3, 77, front.BitDStarLite)
	fmt.Printf("[Такт 3 | growth]  Выведен новый закон Gen#%d: Guard=[%.1f..%.1f], BlackSwans=%d/%d, Alpha=%.1f\n", evoAudit.GenerationID, evoAudit.GuardMin, evoAudit.GuardMax, evoAudit.PassedSwans, evoAudit.TotalSwans, evoAudit.AlphaBlend)

	frontRouter.UpdatePolicyRCU(&front.RouterPolicy{Version: 99})
	id4 := coreBus.Publish(&anomalyHDC, gOff1, tOff2, 100, 10)
	v4, _ := frontRouter.Step(id4)
	fmt.Printf("[Такт 4 | front]   Повтор сложного сбоя: отработан как рефлекс! Confidence=%.2f\n", v4.Confidence)

	busStats := coreBus.Metrics.Snapshot()
	fmt.Println("\n=== ИТОГОВЫЙ ТЕПЛОВОЙ ОТЧЁТ ОРГАНИЗМА (КОН 10) ===")
	fmt.Printf("Шина (Bus):     Опубликовано=%d | Ошибок CRC32=%d | Пропусков эпохи=%d\n", busStats.Published, busStats.ChecksumErrors, busStats.Backpressure)
	fmt.Printf("Слой front:     Surprise=%d | Заблокировано Veto=%d | Эскалаций в growth=%d\n", frontRouter.SurpriseCount.Load(), frontRouter.VetoBlocked.Load(), frontRouter.EscalatedGrowth.Load())
	fmt.Printf("Слой polygon:   Диагнозов=%d | Антител в Карантин=%d | Выдано Veto=%d\n", polygonRouter.DiagnosedCount.Load(), polygonRouter.AntibodiesSent.Load(), polygonRouter.VetoIssuedCount.Load())
	fmt.Printf("Слой growth:    Новых законов=%d | Забраковано=%d | Реабилитаций Veto=%d\n", growthRouter.EvolvedLawsCount.Load(), growthRouter.RejectedLawsCount.Load(), growthRouter.RehabilitatedCount.Load())
	fmt.Printf("Слой stratos:   Записей законов в WAL=%d | Токсичных меток=%d\n", memoryStore.LawsRecorded.Load(), memoryStore.ToxicMarked.Load())

	if os.Getenv("ORGANISM_DAEMON") == "1" {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\n[Shutdown] Получен сигнал остановки, завершаем контуры...")
	}
}
