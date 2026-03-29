package pipeline

// stepUseBrain returns true if brain read injection is active for this step.
// Step-level pointer overrides pipeline-level bool; nil means inherit.
func stepUseBrain(p *Pipeline, s *Step) bool {
	if s.UseBrain != nil {
		return *s.UseBrain
	}
	return p.UseBrain
}

// stepWriteBrain returns true if brain write injection is active for this step.
// Step-level pointer overrides pipeline-level bool; nil means inherit.
func stepWriteBrain(p *Pipeline, s *Step) bool {
	if s.WriteBrain != nil {
		return *s.WriteBrain
	}
	return p.WriteBrain
}
