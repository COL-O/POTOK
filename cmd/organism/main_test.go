package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/growth"
	"organism/modules/polygon"
	"organism/modules/stratos"
)

func TestDashboardStartHealthMetricsAndGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	coreBus, err := bus.NewBus(bus.BusConfig{
		PulseRingSize:   256,
		HDCRingSize:     256,
		GraphArenaSize:  32 * 1024,
		TensorArenaSize: 32 * 1024,
		MetaRingSize:    256,
		ErrorRingSize:   256,
		VectorArenaSize: 16 * 1024,
		AudioArenaSize:  16 * 1024,
		PinFrontInRAM:   false,
	})
	if err != nil {
		t.Fatalf("NewBus failed: %v", err)
	}

	pool, _ := bus.NewScratchPool(8, 1024)
	store, err := stratos.NewStore(filepath.Join(t.TempDir(), "dash_test.wal"), nil)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	var pattern bus.HDCSlot
	hdc := front.NewHDCOrgan([]front.HDCPrototype{
		{Pattern: pattern, ClassID: 1, BaseMask: front.BaseRoutineMask, MinSim: 0.90},
	})
	tsetlin := front.NewTsetlinOrgan([]front.TsetlinClause{
		{IncludeFlags: bus.FlagReady, AddMask: front.BitPasport, ActionCode: 10, Weight: 0.95},
	})

	reg := bus.NewOrganRegistry()
	_ = reg.HotSwap(hdc)
	_ = reg.HotSwap(tsetlin)
	_ = reg.HotSwap(&front.PasportOrgan{})

	frontR := front.NewRouter(front.RouterConfig{
		BaseLeaseDuration: 10 * time.Millisecond,
		MaxFlapsInWindow:  3,
		FlapWindowNs:      int64(time.Second),
	}, coreBus, reg, pool, hdc, tsetlin, nil)

	polyR := polygon.NewRouter(polygon.RouterConfig{}, coreBus, reg, pool, store, nil,
		&polygon.MatrixProfileOrgan{WindowSize: 4, DiscordThresh: 2.0},
		polygon.NewKANOrgan(&polygon.KANRuleTable{Version: 1, MinVal: 0, MaxVal: 10, Knots: []float32{0.1, 0.9}}),
		&polygon.CfCOrgan{}, &polygon.CBSOrgan{}, &polygon.ESNOrgan{},
		polygon.NewPolygonTsetlinOrgan(nil),
	)

	growthR := growth.NewRouter(growth.RouterConfig{}, coreBus, pool, store, nil,
		&growth.GraphSAGEOrgan{}, &growth.ActiveInferenceOrgan{}, &growth.SINDyOrgan{},
		&growth.LUTKANOrgan{}, &growth.GrowthMCTSOrgan{}, &growth.TDMPC2Organ{},
	)

	id := coreBus.Publish(&pattern, bus.NullOffset, bus.NullOffset, 100, 10)
	_, _ = frontR.Step(id)

	dash := NewDashboardServer(addr, coreBus, reg, frontR, polyR, growthR, store)
	if err := dash.Start(); err != nil {
		t.Fatalf("dashboard Start failed: %v", err)
	}

	client := &http.Client{Timeout: 500 * time.Millisecond}
	var resp *http.Response
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resp, err = client.Get("http://" + addr + "/health")
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dashboard /health did not become ready: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /health 200 OK, got %d", resp.StatusCode)
	}

	mResp, err := client.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer mResp.Body.Close()

	var telemetry OrganismTelemetry
	if err := json.NewDecoder(mResp.Body).Decode(&telemetry); err != nil {
		t.Fatalf("failed to decode telemetry JSON: %v", err)
	}
	if telemetry.Bus.Published != 1 || len(telemetry.Heatmap) < 3 {
		t.Fatalf("unexpected telemetry snapshot: %+v", telemetry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dash.Shutdown(ctx); err != nil {
		t.Fatalf("dashboard Shutdown failed: %v", err)
	}
	pool.Close()
	coreBus.Close()
	_ = store.Close()
}
