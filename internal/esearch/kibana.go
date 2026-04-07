package esearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Kibana is a tiny client for the Kibana saved-objects API. It's
// deliberately not the official elastic/kibana SDK — gl1tch only needs
// to (a) check that Kibana is reachable and (b) idempotently create a
// handful of Data Views so the user can open Kibana and immediately see
// glitch's indices charted in Lens. Pulling the full SDK would bloat
// every binary in the repo for what is, at the end of the day, three
// HTTP POSTs.
//
// All operations target the default space ("default"). The data views
// are tagged with deterministic ids (`glitch-*`) so re-running
// EnsureDataViews on every glitchd start is a no-op once they exist.
type Kibana struct {
	addr string
	hc   *http.Client
}

// NewKibana builds a Kibana client. addr defaults to
// http://localhost:5601 (matching docker-compose.yml) when empty. The
// GLITCH_KIBANA_URL env var overrides the default — handy for users
// running Kibana on a non-standard port without editing config.
func NewKibana(addr string) *Kibana {
	if addr == "" {
		addr = os.Getenv("GLITCH_KIBANA_URL")
	}
	if addr == "" {
		addr = "http://localhost:5601"
	}
	return &Kibana{
		addr: addr,
		hc:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Ping returns nil if Kibana's /api/status endpoint reports a 200.
// Used as a cheap reachability probe before any saved-objects calls so
// startup doesn't spend 30s waiting on a dead Kibana when the user
// hasn't started the container.
func (k *Kibana) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", k.addr+"/api/status", nil)
	if err != nil {
		return err
	}
	res, err := k.hc.Do(req)
	if err != nil {
		return fmt.Errorf("kibana: ping: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("kibana: ping: status %d", res.StatusCode)
	}
	return nil
}

// IsAvailable is the boolean form of Ping with its own short-lived
// context, mirroring esearch.Client.IsAvailable so callers can guard
// "open in kibana" buttons in the desktop UI.
func (k *Kibana) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return k.Ping(ctx) == nil
}

// dataViewSpec is the request body shape for POST /api/data_views/data_view.
// We pin id explicitly so subsequent runs hit the "already exists"
// branch instead of creating duplicate views every restart.
type dataViewSpec struct {
	DataView struct {
		ID            string `json:"id"`
		Title         string `json:"title"`
		Name          string `json:"name"`
		TimeFieldName string `json:"timeFieldName,omitempty"`
	} `json:"data_view"`
	Override bool `json:"override"`
}

// dataViewDef is the in-process definition of a data view we want to
// guarantee exists. Title is the index pattern; Name is the human label
// shown in the Kibana sidebar.
type dataViewDef struct {
	ID            string
	Title         string
	Name          string
	TimeFieldName string
}

// glitchDataViews is the canonical set of data views gl1tch expects to
// see in Kibana. Add new entries here when you add a new index — the
// bootstrap loop will create them on the next glitchd restart.
//
// Order matters only insofar as failures are logged in order; each
// view is created independently so one missing index can't block the
// others.
var glitchDataViews = []dataViewDef{
	{
		ID:            "glitch-brain-decisions",
		Title:         "glitch-brain-decisions",
		Name:          "glitch · brain decisions",
		TimeFieldName: "timestamp",
	},
	{
		ID:            "glitch-pipelines",
		Title:         "glitch-pipelines",
		Name:          "glitch · pipeline runs",
		TimeFieldName: "timestamp",
	},
	{
		ID:            "glitch-events",
		Title:         "glitch-events",
		Name:          "glitch · events",
		TimeFieldName: "timestamp",
	},
	{
		ID:            "glitch-summaries",
		Title:         "glitch-summaries",
		Name:          "glitch · summaries",
		TimeFieldName: "timestamp",
	},
	{
		ID:            "glitch-insights",
		Title:         "glitch-insights",
		Name:          "glitch · insights",
		TimeFieldName: "timestamp",
	},
}

// EnsureDataViews creates each Data View in glitchDataViews if it
// doesn't already exist. Idempotent: existing views (HTTP 409 from
// Kibana) are treated as success. Returns the first hard error
// encountered, but always tries every view first so a single broken
// one doesn't hide the others.
//
// Best-effort by design — the caller (glitchd startup) logs and
// continues on failure so a missing/dead Kibana never blocks the
// daemon. Re-running glitchd after starting Kibana will pick up where
// this left off.
func (k *Kibana) EnsureDataViews(ctx context.Context) error {
	if err := k.Ping(ctx); err != nil {
		return err
	}
	var firstErr error
	for _, dv := range glitchDataViews {
		if err := k.createDataView(ctx, dv); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// createDataView POSTs a single data view, treating the
// "already exists" 409 as success. We don't bother with a "get then
// post" round-trip — Kibana's idempotent override:false handles the
// race correctly and saves us a network hop on the common case.
func (k *Kibana) createDataView(ctx context.Context, dv dataViewDef) error {
	var spec dataViewSpec
	spec.DataView.ID = dv.ID
	spec.DataView.Title = dv.Title
	spec.DataView.Name = dv.Name
	spec.DataView.TimeFieldName = dv.TimeFieldName
	spec.Override = false

	body, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		k.addr+"/api/data_views/data_view",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// kbn-xsrf is required by all Kibana write endpoints — without it
	// you get a 400 even on a security-disabled cluster.
	req.Header.Set("kbn-xsrf", "glitch")

	res, err := k.hc.Do(req)
	if err != nil {
		return fmt.Errorf("kibana: create data view %q: %w", dv.ID, err)
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)
	switch res.StatusCode {
	case 200, 201:
		return nil
	case 409:
		// Some Kibana versions return 409 on conflict — treat as success.
		return nil
	case 400:
		// Kibana 8.17 returns 400 with `Duplicate data view: <name>` in
		// the message body when a view with the same title already
		// exists. There's no dedicated conflict status, so we have to
		// match on the message string. Anything else 400 is a real
		// error worth surfacing.
		if strings.Contains(string(raw), "Duplicate data view") {
			return nil
		}
		return fmt.Errorf("kibana: create data view %q: status 400: %s",
			dv.ID, string(raw))
	default:
		return fmt.Errorf("kibana: create data view %q: status %d: %s",
			dv.ID, res.StatusCode, string(raw))
	}
}
