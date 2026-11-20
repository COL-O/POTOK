package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"organism/modules/bus"
	"organism/modules/front"
	"organism/modules/growth"
	"organism/modules/polygon"
	"organism/modules/stratos"
)

type OrganHeatEntry struct {
	ID    uint8  `json:"id"`
	Name  string `json:"name"`
	Layer string `json:"layer"`
	Hits  uint64 `json:"hits"`
}

type OrganismTelemetry struct {
	TimestampNs int64                `json:"timestamp_ns"`
	Bus         bus.BusStatsSnapshot `json:"bus"`
	Front       FrontStats           `json:"front"`
	Polygon     PolygonStats         `json:"polygon"`
	Growth      GrowthStats          `json:"growth"`
	Stratos     StratosStats         `json:"stratos"`
	Heatmap     []OrganHeatEntry     `json:"heatmap"`
}

type FrontStats struct {
	SurpriseCount   uint64 `json:"surprise_count"`
	VetoBlocked     uint64 `json:"veto_blocked"`
	EscalatedGrowth uint64 `json:"escalated_growth"`
	DemotedReflexes uint64 `json:"demoted_reflexes"`
}

type PolygonStats struct {
	DiagnosedCount  uint64 `json:"diagnosed_count"`
	PrecedentHits   uint64 `json:"precedent_hits"`
	AntibodiesSent  uint64 `json:"antibodies_sent"`
	VetoIssuedCount uint64 `json:"veto_issued_count"`
	EscalatedGrowth uint64 `json:"escalated_growth"`
}

type GrowthStats struct {
	EvolvedLawsCount   uint64 `json:"evolved_laws_count"`
	RejectedLawsCount  uint64 `json:"rejected_laws_count"`
	RollbackCount      uint64 `json:"rollback_count"`
	RehabilitatedCount uint64 `json:"rehabilitated_count"`
}

type StratosStats struct {
	RecallQueries   uint64 `json:"recall_queries"`
	RecallHits      uint64 `json:"recall_hits"`
	LawsRecorded    uint64 `json:"laws_recorded"`
	ToxicMarked     uint64 `json:"toxic_marked"`
	SwansRegistered uint64 `json:"swans_registered"`
}

type DashboardServer struct {
	srv      *http.Server
	coreBus  *bus.Bus
	registry *bus.OrganRegistry
	frontR   *front.Router
	polyR    *polygon.Router
	growthR  *growth.Router
	store    *stratos.Store
}

func NewDashboardServer(addr string, b *bus.Bus, reg *bus.OrganRegistry, fr *front.Router, pr *polygon.Router, gr *growth.Router, st *stratos.Store) *DashboardServer {
	d := &DashboardServer{coreBus: b, registry: reg, frontR: fr, polyR: pr, growthR: gr, store: st}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/metrics", d.handleMetrics)
	mux.HandleFunc("/prometheus", d.handlePrometheus)
	d.srv = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	return d
}

func (d *DashboardServer) Start() error {
	ln, err := net.Listen("tcp", d.srv.Addr)
	if err != nil {
		return err
	}
	go func() {
		defer ln.Close()
		_ = d.srv.Serve(ln)
	}()
	return nil
}

func (d *DashboardServer) Shutdown(ctx context.Context) error {
	return d.srv.Shutdown(ctx)
}

func (d *DashboardServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := d.coreBus.Metrics.Snapshot()
	status := "healthy"
	code := http.StatusOK
	if stats.ChecksumErrors > 0 || stats.BoundsErrors > 0 {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          status,
		"published_ticks": stats.Published,
		"checksum_errors": stats.ChecksumErrors,
		"bounds_errors":   stats.BoundsErrors,
	})
}

func (d *DashboardServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	telemetry := d.CollectTelemetry()
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(telemetry)
}

func (d *DashboardServer) handlePrometheus(w http.ResponseWriter, r *http.Request) {
	t := d.CollectTelemetry()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# TYPE organism_bus_published_total counter\norganism_bus_published_total %d\n", t.Bus.Published)
	fmt.Fprintf(w, "# TYPE organism_bus_checksum_errors_total counter\norganism_bus_checksum_errors_total %d\n", t.Bus.ChecksumErrors)
	fmt.Fprintf(w, "# TYPE organism_bus_backpressure_total counter\norganism_bus_backpressure_total %d\n", t.Bus.Backpressure)
	fmt.Fprintf(w, "# TYPE organism_front_surprise_total counter\norganism_front_surprise_total %d\n", t.Front.SurpriseCount)
	fmt.Fprintf(w, "# TYPE organism_front_veto_blocked_total counter\norganism_front_veto_blocked_total %d\n", t.Front.VetoBlocked)
	fmt.Fprintf(w, "# TYPE organism_front_escalated_growth_total counter\norganism_front_escalated_growth_total %d\n", t.Front.EscalatedGrowth)
	fmt.Fprintf(w, "# TYPE organism_polygon_diagnosed_total counter\norganism_polygon_diagnosed_total %d\n", t.Polygon.DiagnosedCount)
	fmt.Fprintf(w, "# TYPE organism_polygon_antibodies_sent_total counter\norganism_polygon_antibodies_sent_total %d\n", t.Polygon.AntibodiesSent)
	fmt.Fprintf(w, "# TYPE organism_growth_evolved_laws_total counter\norganism_growth_evolved_laws_total %d\n", t.Growth.EvolvedLawsCount)
	fmt.Fprintf(w, "# TYPE organism_growth_rollbacks_total counter\norganism_growth_rollbacks_total %d\n", t.Growth.RollbackCount)
	fmt.Fprintf(w, "# TYPE organism_stratos_laws_recorded_total counter\norganism_stratos_laws_recorded_total %d\n", t.Stratos.LawsRecorded)
	fmt.Fprintf(w, "# TYPE organism_stratos_toxic_marked_total counter\norganism_stratos_toxic_marked_total %d\n", t.Stratos.ToxicMarked)
	fmt.Fprintf(w, "# TYPE organism_organ_hits_total counter\n")
	for _, h := range t.Heatmap {
		fmt.Fprintf(w, "organism_organ_hits_total{id=\"%d\",layer=\"%s\",organ=\"%s\"} %d\n", h.ID, h.Layer, h.Name, h.Hits)
	}
}

func (d *DashboardServer) CollectTelemetry() OrganismTelemetry {
	var heatmap []OrganHeatEntry
	for id := uint8(0); id < bus.MaxActiveOrgans; id++ {
		org, ok := d.registry.Get(id)
		if !ok {
			continue
		}
		spec := org.Spec()
		var hits uint64
		switch spec.Layer {
		case bus.LayerFront:
			hits = d.frontR.Heatmap[id].Load()
		case bus.LayerPolygon:
			hits = d.polyR.Heatmap[id].Load()
		case bus.LayerGrowth:
			hits = d.growthR.Heatmap[id].Load()
		}
		heatmap = append(heatmap, OrganHeatEntry{ID: id, Name: spec.Name, Layer: layerName(spec.Layer), Hits: hits})
	}
	sort.Slice(heatmap, func(i, j int) bool {
		if heatmap[i].Hits == heatmap[j].Hits {
			return heatmap[i].ID < heatmap[j].ID
		}
		return heatmap[i].Hits > heatmap[j].Hits
	})
	return OrganismTelemetry{
		TimestampNs: time.Now().UnixNano(),
		Bus:         d.coreBus.Metrics.Snapshot(),
		Front: FrontStats{
			SurpriseCount:   d.frontR.SurpriseCount.Load(),
			VetoBlocked:     d.frontR.VetoBlocked.Load(),
			EscalatedGrowth: d.frontR.EscalatedGrowth.Load(),
			DemotedReflexes: d.frontR.DemotedReflexes.Load(),
		},
		Polygon: PolygonStats{
			DiagnosedCount:  d.polyR.DiagnosedCount.Load(),
			PrecedentHits:   d.polyR.PrecedentHits.Load(),
			AntibodiesSent:  d.polyR.AntibodiesSent.Load(),
			VetoIssuedCount: d.polyR.VetoIssuedCount.Load(),
			EscalatedGrowth: d.polyR.EscalatedGrowth.Load(),
		},
		Growth: GrowthStats{
			EvolvedLawsCount:   d.growthR.EvolvedLawsCount.Load(),
			RejectedLawsCount:  d.growthR.RejectedLawsCount.Load(),
			RollbackCount:      d.growthR.RollbackCount.Load(),
			RehabilitatedCount: d.growthR.RehabilitatedCount.Load(),
		},
		Stratos: StratosStats{
			RecallQueries:   d.store.RecallQueries.Load(),
			RecallHits:      d.store.RecallHits.Load(),
			LawsRecorded:    d.store.LawsRecorded.Load(),
			ToxicMarked:     d.store.ToxicMarked.Load(),
			SwansRegistered: d.store.SwansRegistered.Load(),
		},
		Heatmap: heatmap,
	}
}

func layerName(l bus.LayerID) string {
	switch l {
	case bus.LayerFront:
		return "front"
	case bus.LayerPolygon:
		return "polygon"
	case bus.LayerGrowth:
		return "growth"
	case bus.LayerStratos:
		return "stratos"
	default:
		return "unknown"
	}
}
