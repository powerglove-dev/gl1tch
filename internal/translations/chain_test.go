package translations_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/translations"
)

// ── ChainProvider ─────────────────────────────────────────────────────────────

func TestChain_UserWinsOverTheme(t *testing.T) {
	user := translations.NewMapProvider(map[string]string{"k": "user-value"})
	theme := translations.NewMapProvider(map[string]string{"k": "theme-value"})
	defaults := translations.NewMapProvider(map[string]string{"k": "default-value"})

	chain := translations.NewChain(user, theme, defaults)
	if got := chain.T("k", "fallback"); got != "user-value" {
		t.Errorf("got %q, want user-value", got)
	}
}

func TestChain_ThemeWinsOverDefaults(t *testing.T) {
	user := translations.NewMapProvider(map[string]string{})
	theme := translations.NewMapProvider(map[string]string{"k": "theme-value"})
	defaults := translations.NewMapProvider(map[string]string{"k": "default-value"})

	chain := translations.NewChain(user, theme, defaults)
	if got := chain.T("k", "fallback"); got != "theme-value" {
		t.Errorf("got %q, want theme-value", got)
	}
}

func TestChain_DefaultsWinOverFallback(t *testing.T) {
	user := translations.NewMapProvider(map[string]string{})
	theme := translations.NewMapProvider(map[string]string{})
	defaults := translations.NewMapProvider(map[string]string{"k": "default-value"})

	chain := translations.NewChain(user, theme, defaults)
	if got := chain.T("k", "fallback"); got != "default-value" {
		t.Errorf("got %q, want default-value", got)
	}
}

func TestChain_FallbackWhenNoProviderHasKey(t *testing.T) {
	chain := translations.NewChain(
		translations.NewMapProvider(map[string]string{}),
		translations.NewMapProvider(map[string]string{}),
	)
	if got := chain.T("missing", "the-fallback"); got != "the-fallback" {
		t.Errorf("got %q, want the-fallback", got)
	}
}

func TestChain_NilProviderSkipped(t *testing.T) {
	good := translations.NewMapProvider(map[string]string{"k": "good"})
	chain := translations.NewChain(nil, good)
	if got := chain.T("k", "fallback"); got != "good" {
		t.Errorf("got %q, want good", got)
	}
}

func TestChain_EmptyChainReturnsHardFallback(t *testing.T) {
	chain := translations.NewChain()
	if got := chain.T("k", "raw"); got != "raw" {
		t.Errorf("got %q, want raw", got)
	}
}

// ── MapProvider ───────────────────────────────────────────────────────────────

func TestMapProvider_KnownKey(t *testing.T) {
	p := translations.NewMapProvider(map[string]string{"x": "hello"})
	if got := p.T("x", "fallback"); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestMapProvider_MissingKeyReturnsFallback(t *testing.T) {
	p := translations.NewMapProvider(map[string]string{})
	if got := p.T("missing", "fb"); got != "fb" {
		t.Errorf("got %q, want fb", got)
	}
}

func TestMapProvider_ANSIEscapeExpansion(t *testing.T) {
	p := translations.NewMapProvider(map[string]string{"c": `\e[0m`})
	if got := p.T("c", ""); got != "\x1b[0m" {
		t.Errorf("got %q, want ESC[0m", got)
	}
}

func TestMapProvider_NilMapReturnsNop(t *testing.T) {
	p := translations.NewMapProvider(nil)
	if got := p.T("any", "nop-result"); got != "nop-result" {
		t.Errorf("got %q, want nop-result", got)
	}
}

// ── DefaultProvider ───────────────────────────────────────────────────────────

func TestDefaultProvider_HasQuitMessage(t *testing.T) {
	p := translations.NewDefaultProvider()
	got := p.T(translations.KeyQuitConfirmMessage, "")
	if got == "" {
		t.Error("default quit message should not be empty")
	}
}

func TestDefaultProvider_HasWelcomeIntro(t *testing.T) {
	p := translations.NewDefaultProvider()
	got := p.T(translations.KeyWelcomePhaseIntro, "")
	if got == "" {
		t.Error("default welcome intro should not be empty")
	}
}

func TestDefaultProvider_AllPanelTitlesPresent(t *testing.T) {
	p := translations.NewDefaultProvider()
	keys := []string{
		translations.KeyPipelinesTitle,
		translations.KeyAgentRunnerTitle,
		translations.KeySignalBoardTitle,
		translations.KeyActivityFeedTitle,
		translations.KeyInboxTitle,
		translations.KeyCronTitle,
	}
	for _, k := range keys {
		if got := p.T(k, ""); got == "" {
			t.Errorf("default provider missing panel title key %q", k)
		}
	}
}

// ── RebuildChain ──────────────────────────────────────────────────────────────

func TestRebuildChain_ThemeStringsOverrideDefaults(t *testing.T) {
	translations.RebuildChain(map[string]string{
		translations.KeyQuitModalTitle: "theme-quit-title",
	})
	p := translations.GlobalProvider()
	if p == nil {
		t.Fatal("GlobalProvider is nil after RebuildChain")
	}
	got := p.T(translations.KeyQuitModalTitle, "fallback")
	if got != "theme-quit-title" {
		t.Errorf("got %q, want theme-quit-title", got)
	}
}

func TestRebuildChain_NilThemeStringsUsesDefaults(t *testing.T) {
	translations.RebuildChain(nil)
	p := translations.GlobalProvider()
	if p == nil {
		t.Fatal("GlobalProvider is nil after RebuildChain(nil)")
	}
	got := p.T(translations.KeyQuitConfirmMessage, "fallback")
	if got == "fallback" {
		t.Error("expected default provider value, got hard fallback")
	}
}
