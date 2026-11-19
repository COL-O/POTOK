package main

import (
	"fmt"
	"log"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
)

const (
	OrganIDDroneAcoustic uint8  = 24
	BitDroneAcoustic     uint32 = 1 << OrganIDDroneAcoustic
)

type DroneAcousticOrgan struct {
	ShockThreshold int16
}

func (o *DroneAcousticOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{
		ID:          OrganIDDroneAcoustic,
		Name:        "DroneAcoustic",
		Layer:       bus.LayerFront,
		ReadLanes:   bus.LaneMask(0).With(bus.LanePulse).With(bus.LaneAudio),
		WriteLane:   bus.LanePulse,
		Residency:   bus.ResidencyPinned,
		SoftTimeout: 5 * time.Microsecond,
		HardTimeout: 20 * time.Microsecond,
	}
}

func (o *DroneAcousticOrgan) InBounds(view bus.TickView) bool { return len(view.Audio) > 0 }
func (o *DroneAcousticOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *DroneAcousticOrgan) CoolDown()                        {}

func (o *DroneAcousticOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	start := time.Now().UnixNano()
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusAbstain},
			bus.OrganMetrics{LatencyNs: uint64(time.Now().UnixNano() - start), Status: bus.StatusAbstain}
	}
	var peak int16
	for _, s := range view.Audio {
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
	}
	lat := uint64(time.Now().UnixNano() - start)
	if peak >= o.ShockThreshold {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.96, ActionCode: 707, SuggestedMask: front.BitDStarLite | front.BitMCTS},
			bus.OrganMetrics{LatencyNs: lat, Confidence: 0.96, Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.92, ActionCode: 100},
		bus.OrganMetrics{LatencyNs: lat, Confidence: 0.92, Status: bus.StatusOK}
}

func main() {
	fmt.Println("=== ЗАПУСК ПРОФИЛЯ АВТОНОМНОГО ДРОНА (КОН 8: ДЕЛЕНИЕ) ===")

	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   512,
		HDCRingSize:     512,
		GraphArenaSize:  32 * 1024,
		TensorArenaSize: 32 * 1024,
		MetaRingSize:    512,
		ErrorRingSize:   256,
		VectorArenaSize: 16 * 1024,
		AudioArenaSize:  64 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		log.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	pool, _ := bus.NewScratchPool(16, 2048)
	defer pool.Close()

	var flightPattern bus.HDCSlot
	flightPattern.Words[0] = 0xD00D00D00D00D001

	droneMask := front.BaseRoutineMask | BitDroneAcoustic | front.BitDStarLite | front.BitMCTS

	hdc := front.NewHDCOrgan([]front.HDCPrototype{
		{Pattern: flightPattern, ClassID: 10, BaseMask: droneMask, MinSim: 0.85},
	})
	tsetlin := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: BitDroneAcoustic | front.BitDStarLite, ActionCode: 200, Weight: 0.90},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(tsetlin)
	_ = reg.HotSwap(&front.DStarLiteOrgan{})
	_ = reg.HotSwap(&front.MCTSOrgan{MaxRollouts: 8})
	_ = reg.HotSwap(&front.PasportOrgan{})
	_ = reg.HotSwap(&DroneAcousticOrgan{ShockThreshold: 15000})

	router := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 15 * time.Millisecond,
		MaxFlapsInWindow:  4,
		FlapWindowNs:      int64(time.Second),
	}, coreBus, reg, pool, hdc, tsetlin, nil)

	gOff, _ := coreBus.Graphs.Write(0, []uint32{0, 2, 2, 2}, []uint32{1, 2}, []float32{9.5, 1.2})

	calmAudio := []int16{1200, -1800, 3200, -2100, 1500}
	id1 := coreBus.Publish(&flightPattern, gOff, bus.NullOffset, 100, 50)
	view1, _ := coreBus.ViewFor(id1, bus.LaneMask(0).With(bus.LanePulse), nil)
	view1.Audio = calmAudio
	v1, _ := router.Step(id1)
	fmt.Printf("[Полёт 1] Штатный режим: Status=%d, Confidence=%.2f, Вызовов DroneAcoustic=%d\n", v1.Status, v1.Confidence, router.Heatmap[OrganIDDroneAcoustic].Load())

	shockAudio := []int16{1400, 24000, -19500, 4100}
	droneOrg, _ := reg.Get(OrganIDDroneAcoustic)
	vAudio, _ := droneOrg.Process(bus.TickView{EventID: 2, Audio: shockAudio})
	fmt.Printf("[Полёт 2] Акустический удар: ActionCode=%d (Уклонение), Confidence=%.2f, NextMask=%#x\n", vAudio.ActionCode, vAudio.Confidence, vAudio.SuggestedMask)
}
