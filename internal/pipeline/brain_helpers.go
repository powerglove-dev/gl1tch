package pipeline

// stepWriteBrain returns true if brain write injection is active for this step.
// Step-level pointer overrides pipeline-level bool; nil means inherit.
func stepWriteBrain(p *Pipeline, s *Step) bool {
	if s.WriteBrain != nil {
		return *s.WriteBrain
	}
	return p.WriteBrain
}
