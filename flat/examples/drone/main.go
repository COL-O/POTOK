package main

import (
	"fmt"
	"log"
	"time"

	"organism/bus"
	"organism/front"
)

const (
	OrganIDDroneAcoustic uint8  = 24
	BitDroneAcoustic     uint32 = 1 << OrganIDDroneAcoustic
)

type DroneAcousticOrgan struct{ ShockThreshold int16 }

func (o *DroneAcousticOrgan) Spec() bus.OrganSpec {
	return bus.OrganSpec{ID: OrganIDDroneAcoustic, Name: "DroneAcoustic", Layer: bus.LayerFront, Residency: bus.ResidencyPinned}
}
func (o *DroneAcousticOrgan) InBounds(view bus.TickView) bool { return len(view.Audio) > 0 }
func (o *DroneAcousticOrgan) Warmup(*bus.MmapRegion) error     { return nil }
func (o *DroneAcousticOrgan) CoolDown()                        {}
func (o *DroneAcousticOrgan) Process(view bus.TickView) (bus.Verdict, bus.OrganMetrics) {
	if !o.InBounds(view) {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusAbstain}, bus.OrganMetrics{Status: bus.StatusAbstain}
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
	if peak >= o.ShockThreshold {
		return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.96, ActionCode: 707}, bus.OrganMetrics{Status: bus.StatusOK}
	}
	return bus.Verdict{OrganID: OrganIDDroneAcoustic, Status: bus.StatusOK, Confidence: 0.92, ActionCode: 100}, bus.OrganMetrics{Status: bus.StatusOK}
}

func main() {
	coreBus, err := bus.NewBus(bus.BusConfig{PulseRingSize: 256, HDCRingSize: 256, GraphArenaSize: 16 * 1024, TensorArenaSize: 16 * 1024})
	if err != nil {
		log.Fatalf("NewBus failed: %v", err)
	}
	defer coreBus.Close()

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(&DroneAcousticOrgan{ShockThreshold: 15000})
	_ = reg.HotSwap(&front.DStarLiteOrgan{})

	droneOrg, _ := reg.Get(OrganIDDroneAcoustic)
	vAudio, _ := droneOrg.Process(bus.TickView{EventID: 1, Audio: []int16{1400, 24000, -19500, 4100}})
	fmt.Printf("[Полёт 2] Акустический удар: ActionCode=%d (Уклонение), Confidence=%.2f, Время=%s\n",
		vAudio.ActionCode, vAudio.Confidence, time.Now().Format("15:04:05"))
}
