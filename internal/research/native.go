package research

import (
	"fmt"

	"github.com/8op-org/gl1tch/internal/capability"
)

// RegisterNatives walks a capability.Registry and registers a
// CapabilityResearcher for every capability whose Name matches one of the
// `wanted` identifiers. Capabilities not in the wanted list are ignored —
// the loop's default native set is intentionally small (git, esearch,
// observer, brainrag) so the planner sees a stable, well-described menu
// rather than every capability the workspace happens to have loaded.
//
// Pass an empty `wanted` slice to register every capability in the source
// registry. This is convenient for tests and for power users who want the
// loop to consider every capability the workspace exposes.
//
// Errors from individual registrations (most often duplicate-name conflicts)
// are returned as a joined error so the caller can decide whether to abort
// startup or continue with a partial registration. The default behavior of
// the calling code at startup should be to log and continue — a missing
// researcher is a degraded loop, not a fatal one.
func RegisterNatives(reg *Registry, source *capability.Registry, wanted ...string) error {
	if reg == nil {
		return fmt.Errorf("research: RegisterNatives: nil destination registry")
	}
	if source == nil {
		return fmt.Errorf("research: RegisterNatives: nil source capability registry")
	}

	var (
		names []string
		all   = len(wanted) == 0
	)
	if all {
		names = source.Names()
	} else {
		names = wanted
	}

	var errs []error
	for _, name := range names {
		cap, ok := source.Get(name)
		if !ok {
			if !all {
				errs = append(errs, fmt.Errorf("research: RegisterNatives: capability %q not found in source registry", name))
			}
			continue
		}
		researcher := NewCapabilityResearcher(cap)
		if researcher == nil {
			continue
		}
		if err := reg.Register(researcher); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrs(errs)
}

// DefaultNativeNames is the canonical list of capability names the research
// loop wraps as native researchers. Listed here so that the assistant
// bootstrap, tests, and the eventual `glitch researcher list` command all
// agree on what "native" means.
//
// Adding to this list requires a corresponding capability to exist in
// internal/capability with that exact Name. The loop tolerates missing
// entries — see RegisterNatives — so a workspace that only has some of
// these capabilities still gets a useful (smaller) menu.
var DefaultNativeNames = []string{
	"git",
	"esearch",
	"observer",
	"brainrag",
}

func joinErrs(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	return errorsJoin(errs)
}
