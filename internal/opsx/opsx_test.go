package opsx_test

import (
	"testing"

	"github.com/8op-org/gl1tch/internal/opsx"
)

func TestFeatureSlug_Simple(t *testing.T) {
	got := opsx.FeatureSlug("My Cool Feature")
	want := "my-cool-feature"
	if got != want {
		t.Errorf("FeatureSlug(%q) = %q, want %q", "My Cool Feature", got, want)
	}
}

func TestFeatureSlug_AlreadySlug(t *testing.T) {
	got := opsx.FeatureSlug("my-feature")
	want := "my-feature"
	if got != want {
		t.Errorf("FeatureSlug(%q) = %q, want %q", "my-feature", got, want)
	}
}

func TestFeatureSlug_EmptyReturnsEmpty(t *testing.T) {
	got := opsx.FeatureSlug("")
	if got != "" {
		t.Errorf("FeatureSlug(%q) = %q, want empty", "", got)
	}
}

func TestFeatureSlug_Whitespace(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"  my feature  ", "my-feature"},
		{"add\tlogin", "add-login"},
		{"add  login", "add-login"},
	}
	for _, c := range cases {
		got := opsx.FeatureSlug(c.input)
		if got != c.want {
			t.Errorf("FeatureSlug(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
