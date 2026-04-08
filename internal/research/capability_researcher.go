package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/8op-org/gl1tch/internal/capability"
)

// CapabilityResearcher is a thin adapter that exposes any registered
// capability.Capability as a Researcher. It performs no model calls of its
// own — Gather() simply invokes the underlying capability with the query as
// the input string and aggregates the resulting events into a single
// Evidence value.
//
// Stream events become the Body of the returned Evidence, concatenated in
// order. Doc events are JSON-marshalled and stored under Meta["doc.<n>"] so
// the draft stage can cite them as structured references. Error events are
// captured separately and surface as a non-nil error from Gather() once the
// channel closes — the loop's gather stage handles the partial-bundle
// behaviour, so a researcher error here never crashes the loop.
type CapabilityResearcher struct {
	cap          capability.Capability
	displayName  string
	displayDescr string
}

// NewCapabilityResearcher wraps a capability as a Researcher. The
// researcher's Name and Describe are taken from the capability's Manifest.
// Pass overrides via the option functions to provide a different name or
// description (useful when one capability is exposed under several
// researcher identities).
func NewCapabilityResearcher(c capability.Capability, opts ...CapabilityOption) *CapabilityResearcher {
	if c == nil {
		return nil
	}
	m := c.Manifest()
	r := &CapabilityResearcher{
		cap:          c,
		displayName:  m.Name,
		displayDescr: m.Description,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// CapabilityOption customises a CapabilityResearcher. Reserved for cases
// where one capability needs to be registered under multiple researcher
// identities — keep this surface small.
type CapabilityOption func(*CapabilityResearcher)

// WithName overrides the researcher's name. Use sparingly; the planner is
// easier to debug when researcher names match capability names.
func WithName(name string) CapabilityOption {
	return func(r *CapabilityResearcher) { r.displayName = name }
}

// WithDescribe overrides the researcher's description shown to the planner.
func WithDescribe(describe string) CapabilityOption {
	return func(r *CapabilityResearcher) { r.displayDescr = describe }
}

// Name implements Researcher.
func (r *CapabilityResearcher) Name() string { return r.displayName }

// Describe implements Researcher.
func (r *CapabilityResearcher) Describe() string { return r.displayDescr }

// Gather implements Researcher.
func (r *CapabilityResearcher) Gather(ctx context.Context, q ResearchQuery, _ EvidenceBundle) (Evidence, error) {
	if r.cap == nil {
		return Evidence{}, errors.New("research: nil capability in CapabilityResearcher")
	}
	in := capability.Input{
		Stdin: q.Question,
		Vars:  flattenContext(q.Context),
	}
	events, err := r.cap.Invoke(ctx, in)
	if err != nil {
		return Evidence{}, fmt.Errorf("invoke %q: %w", r.displayName, err)
	}

	var (
		body strings.Builder
		errs []error
		meta = make(map[string]string)
		refs []string
		docN int
	)

drain:
	for {
		select {
		case <-ctx.Done():
			return Evidence{
				Source: r.displayName,
				Title:  fmt.Sprintf("%s (cancelled)", r.displayName),
				Body:   body.String(),
				Meta:   meta,
				Refs:   refs,
			}, ctx.Err()
		case ev, ok := <-events:
			if !ok {
				break drain
			}
			switch ev.Kind {
			case capability.EventStream:
				body.WriteString(ev.Text)
			case capability.EventDoc:
				if ev.Doc == nil {
					continue
				}
				encoded, mErr := json.Marshal(ev.Doc)
				if mErr != nil {
					errs = append(errs, fmt.Errorf("marshal doc event: %w", mErr))
					continue
				}
				docN++
				meta[fmt.Sprintf("doc.%d", docN)] = string(encoded)
				if ref := docRef(ev.Doc); ref != "" {
					refs = append(refs, ref)
				}
			case capability.EventError:
				if ev.Err != nil {
					errs = append(errs, ev.Err)
				}
			}
		}
	}

	ev := Evidence{
		Source: r.displayName,
		Title:  fmt.Sprintf("%s output", r.displayName),
		Body:   body.String(),
		Refs:   refs,
		Meta:   meta,
	}
	if len(errs) > 0 {
		return ev, errors.Join(errs...)
	}
	return ev, nil
}

// flattenContext copies a query context into the capability Vars map. Both
// types are map[string]string today; the indirection lets us evolve
// ResearchQuery.Context without breaking the capability surface.
func flattenContext(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// docRef tries to extract a single human-readable reference from a Doc event
// payload — a path, URL, or commit SHA. The heuristic is intentionally cheap
// (look for common field names on a JSON object); if nothing matches, the
// caller falls back to the full marshalled doc in Meta.
func docRef(doc any) string {
	encoded, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	var probe map[string]any
	if err := json.Unmarshal(encoded, &probe); err != nil {
		return ""
	}
	for _, key := range []string{"url", "path", "ref", "sha", "id"} {
		if v, ok := probe[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
