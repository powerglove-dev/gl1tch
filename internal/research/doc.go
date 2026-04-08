// Package research is gl1tch's bounded iterative research primitive.
//
// A research call takes a question and a budget, and runs a fixed five-stage
// loop until either the local model is confident enough to accept its draft,
// the budget is exhausted, or the caller has authorised an escalation to a
// paid model. The five stages are:
//
//	plan     — ask the local model to break the question into evidence needs,
//	           each tagged with the name of a registered Researcher.
//	gather   — invoke the named researchers (in parallel where possible) and
//	           merge their results into an EvidenceBundle.
//	draft    — ask the local model to write an answer that is grounded in the
//	           bundle and only the bundle.
//	critique — a separate local-model pass that labels each claim in the
//	           draft as grounded / partially-grounded / unsupported and emits
//	           a structured per-claim critique.
//	score    — combine self-consistency, evidence coverage, cross-capability
//	           agreement, and a judge pass into a composite confidence in
//	           [0,1]. The loop accepts on threshold, refines and retries on
//	           insufficient evidence, or escalates if the caller allowed it.
//
// The package is intentionally orthogonal to the assistant router. The router
// can register the loop as a capability, but research.Run is callable from
// any goroutine — pipeline steps, tests, future chat widgets — without going
// through the router or the assistant package.
//
// All LLM calls go through the existing pipeline.Run path with executor
// "ollama" and qwen2.5:7b as the default model. The package never constructs
// shell commands, never reaches for keyword tables, and never picks
// confidence weights from intuition: every score signal is logged, so the
// brain can later learn weights from real accept/reject events.
package research
