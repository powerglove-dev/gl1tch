package telemetry

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/8op-org/gl1tch/internal/esearch"
)

// elasticsearchExporter implements sdktrace.SpanExporter by
// transforming each ReadOnlySpan into a flat JSON document and
// bulk-indexing it into the glitch-traces index.
//
// Why a custom exporter instead of OTLP gRPC + APM Server?
//   - gl1tch's ES instance has no APM Server in front of it (see
//     docker-compose.yml — only es + kibana). The native ES OTLP
//     ingest path requires APM Server bridging the OTel data model
//     to the traces-apm-* data streams.
//   - The audit doc-shape we want for "show me what just happened
//     in this process" queries is much flatter than traces-apm-*:
//     trace_id + span_id + name + start/end + attributes is enough
//     to answer 95% of the debugging questions, and Kibana
//     Discover handles it natively without any APM UI plumbing.
//   - Reusing the existing esearch.Client means one connection pool
//     and one auth surface for ES across all of gl1tch.
//
// The exporter is non-blocking on ES outages: ExportSpans builds
// the doc batch and hands it to the ES bulk API with a 5s
// per-call timeout. If ES is down, the call fails and the spans
// are dropped on the floor — we explicitly do NOT queue spans
// indefinitely because the OTel SDK's BatchSpanProcessor already
// has its own bounded queue and back-pressure semantics.
type elasticsearchExporter struct {
	es      *esearch.Client
	mu      sync.Mutex
	stopped bool

	// pidStr is captured once at construction so every doc batch
	// gets the same process_pid without re-querying os.Getpid().
	pidStr  string
	pid     int
	host    string
	service string
}

// NewElasticsearchExporter constructs an exporter that ships spans
// to the given ES client's glitch-traces index. serviceName is
// stamped on every doc as the service field; pass the same value
// you used for telemetry.Setup so traces and logs agree.
func NewElasticsearchExporter(es *esearch.Client, serviceName string) *elasticsearchExporter {
	host, _ := os.Hostname()
	pid := os.Getpid()
	return &elasticsearchExporter{
		es:      es,
		pid:     pid,
		pidStr:  fmt.Sprintf("%d", pid),
		host:    host,
		service: serviceName,
	}
}

// ExportSpans transforms a batch of spans into ES documents and
// bulk-indexes them. Implements sdktrace.SpanExporter.
//
// We swallow the ES error (after logging is left to the caller via
// the SpanExporter interface contract) because the SDK doesn't
// retry on exporter errors and the live trace stream is too noisy
// to print every "ES temporarily unavailable" to stderr. Spans
// already landed in the file exporter as a backstop, so dropping
// the ES copy on a transient outage doesn't lose history.
func (e *elasticsearchExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	stopped := e.stopped
	e.mu.Unlock()
	if stopped {
		return nil
	}
	if len(spans) == 0 {
		return nil
	}

	docs := make([]any, 0, len(spans))
	for _, sp := range spans {
		docs = append(docs, e.spanToDoc(sp))
	}

	// 5s ceiling so a stuck ES doesn't wedge the OTel pipeline. The
	// SpanExporter contract is "best effort"; we explicitly degrade
	// rather than backpressure into the SDK.
	bulkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return e.es.BulkIndex(bulkCtx, esearch.IndexTraces, docs)
}

// Shutdown marks the exporter as stopped so subsequent ExportSpans
// calls become no-ops. The underlying ES client is owned by the
// caller (telemetry.Setup) and not closed here.
func (e *elasticsearchExporter) Shutdown(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	return nil
}

// spanToDoc flattens an OTel ReadOnlySpan into the document shape
// matching tracesMapping in internal/esearch/mappings.go.
//
// Special-cased attribute promotions:
//   - workspace_id and collector are pulled out of attributes into
//     top-level keyword fields so the most common slice queries
//     (group spans by workspace, group by collector) don't have to
//     scan the disabled attributes object.
//   - Resource attributes (service.name, service.version, host.name,
//     process.pid) are flattened to top-level fields too so dashboard
//     filters work without joining against the resource object.
func (e *elasticsearchExporter) spanToDoc(sp sdktrace.ReadOnlySpan) map[string]any {
	sc := sp.SpanContext()
	parent := sp.Parent()

	doc := map[string]any{
		"trace_id":       sc.TraceID().String(),
		"span_id":        sc.SpanID().String(),
		"parent_span_id": parent.SpanID().String(),
		"name":           sp.Name(),
		"kind":           sp.SpanKind().String(),
		"start_time":     sp.StartTime().UTC().Format(time.RFC3339Nano),
		"end_time":       sp.EndTime().UTC().Format(time.RFC3339Nano),
		"duration_ms":    sp.EndTime().Sub(sp.StartTime()).Milliseconds(),
		"status_code":    sp.Status().Code.String(),
		"status_message": sp.Status().Description,
		"process_pid":    e.pid,
		"host_name":      e.host,
		"service_name":   e.service,
	}

	if scope := sp.InstrumentationScope(); scope.Name != "" {
		doc["scope_name"] = scope.Name
	}

	// Flatten span attributes into a map and pull out promoted
	// fields. We use map[string]any (not the OTel KeyValue slice)
	// so the JSON shape matches what Kibana expects out of the box.
	attrs := make(map[string]any, len(sp.Attributes()))
	for _, kv := range sp.Attributes() {
		key := string(kv.Key)
		val := kv.Value.AsInterface()
		switch key {
		case "workspace_id", "workspace.id":
			doc["workspace_id"] = val
		case "collector", "collector.name":
			doc["collector"] = val
		}
		attrs[key] = val
	}
	if len(attrs) > 0 {
		doc["attributes"] = attrs
	}

	// Resource attributes — service name/version, host info, process pid
	// from semconv. Flattened so dashboards can group by them without
	// joining against a nested object.
	if res := sp.Resource(); res != nil {
		resAttrs := make(map[string]any, res.Len())
		for _, kv := range res.Attributes() {
			key := string(kv.Key)
			val := kv.Value.AsInterface()
			switch key {
			case "service.version":
				doc["service_version"] = val
			case "host.name":
				if doc["host_name"] == "" {
					doc["host_name"] = val
				}
			}
			resAttrs[key] = val
		}
		if len(resAttrs) > 0 {
			doc["resource"] = resAttrs
		}
	}

	// Span events — flatten name + attributes per event so a single
	// span doc carries the complete narrative. Capped at 16 events
	// per span so a runaway log doesn't blow up the doc size.
	if events := sp.Events(); len(events) > 0 {
		const maxEvents = 16
		out := make([]map[string]any, 0, len(events))
		for i, ev := range events {
			if i >= maxEvents {
				break
			}
			ea := make(map[string]any, len(ev.Attributes))
			for _, kv := range ev.Attributes {
				ea[string(kv.Key)] = kv.Value.AsInterface()
			}
			out = append(out, map[string]any{
				"name":       ev.Name,
				"time":       ev.Time.UTC().Format(time.RFC3339Nano),
				"attributes": ea,
			})
		}
		doc["events"] = out
	}

	return doc
}
