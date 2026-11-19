package main

import (
	"fmt"
	"log"
	"time"

	"organism/bus"
	"organism/front"
)

func main() {
	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   256,
		HDCRingSize:     256,
		GraphArenaSize:  16 * 1024,
		TensorArenaSize: 16 * 1024,
	})
	if err != nil {
		log.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	var knownPattern bus.HDCSlot
	knownPattern.Words[0] = 0x1111222233334444

	hdc := front.NewHDCOrgan([]front.HDCPrototype{
		{Pattern: knownPattern, ClassID: 1, BaseMask: front.BaseRoutineMask, MinSim: 0.90},
	})
	tsetlin := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 42, Weight: 0.99},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(tsetlin)
	_ = reg.HotSwap(&front.PasportOrgan{})

	router := front.NewRouter(front.RouterConfig{BaseLeaseDuration: 10 * time.Millisecond}, coreBus, reg, nil, hdc, tsetlin, nil)

	id1 := coreBus.Publish(&knownPattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v1, _ := router.Step(id1)
	fmt.Printf("Знакомый импульс: Status=%d, Action=%d, Confidence=%.2f\n", v1.Status, v1.ActionCode, v1.Confidence)

	var unknownPattern bus.HDCSlot
	for i := range unknownPattern.Words {
		unknownPattern.Words[i] = ^uint64(0)
	}
	id2 := coreBus.Publish(&unknownPattern, bus.NullOffset, bus.NullOffset, 100, 10)
	v2, _ := router.Step(id2)
	fmt.Printf("Неизвестный импульс: Status=%d, Surprises=%d\n", v2.Status, router.SurpriseCount.Load())
}
