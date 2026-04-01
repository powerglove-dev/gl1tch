package themes_test

import (
	"testing"
	"time"

	"github.com/8op-org/gl1tch/internal/themes"
)

// resetGlobal clears the global singleton between tests using SetGlobalRegistry(nil).
func resetGlobal() {
	themes.SetGlobalRegistry(nil)
}

// TestGlobalRegistrySamePointer verifies that multiple calls to GlobalRegistry
// return the same pointer before and after SetGlobalRegistry.
func TestGlobalRegistrySamePointer(t *testing.T) {
	resetGlobal()

	// Before SetGlobalRegistry: should be nil.
	if got := themes.GlobalRegistry(); got != nil {
		t.Fatalf("expected nil before SetGlobalRegistry, got %v", got)
	}

	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	themes.SetGlobalRegistry(reg)

	// After: both calls should return the same pointer.
	g1 := themes.GlobalRegistry()
	g2 := themes.GlobalRegistry()
	if g1 == nil {
		t.Fatal("GlobalRegistry() returned nil after SetGlobalRegistry")
	}
	if g1 != g2 {
		t.Fatalf("GlobalRegistry() returned different pointers: %p vs %p", g1, g2)
	}
	if g1 != reg {
		t.Fatalf("GlobalRegistry() returned %p, want %p", g1, reg)
	}

	resetGlobal()
}

// TestSetGlobalRegistryOverwrite verifies that SetGlobalRegistry can replace
// the stored registry (the global is a plain pointer, not a sync.Once value).
func TestSetGlobalRegistryOverwrite(t *testing.T) {
	resetGlobal()

	reg1, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	themes.SetGlobalRegistry(reg1)

	reg2, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	themes.SetGlobalRegistry(reg2)

	if got := themes.GlobalRegistry(); got != reg2 {
		t.Fatalf("after second SetGlobalRegistry: got %p, want %p", got, reg2)
	}

	resetGlobal()
}

// TestSubscribeReceivesNotification verifies that a subscribed channel receives
// the new theme name when SetActive is called.
func TestSubscribeReceivesNotification(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Pick a theme to switch to (use whatever is available).
	all := reg.All()
	if len(all) < 2 {
		t.Skip("need at least 2 themes to test subscription notification")
	}

	// Subscribe before calling SetActive.
	ch := make(chan string, 1)
	reg.Subscribe(ch)

	target := all[0].Name
	if reg.Active() != nil && reg.Active().Name == target {
		target = all[1].Name
	}

	if err := reg.SetActive(target); err != nil {
		t.Fatalf("SetActive(%q): %v", target, err)
	}

	select {
	case got := <-ch:
		if got != target {
			t.Fatalf("subscriber got %q, want %q", got, target)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber notification")
	}
}

// TestUnsubscribeReceivesNothing verifies that an unsubscribed channel does
// not receive notifications after Unsubscribe is called.
func TestUnsubscribeReceivesNothing(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	all := reg.All()
	if len(all) < 2 {
		t.Skip("need at least 2 themes to test unsubscribe")
	}

	ch := make(chan string, 1)
	reg.Subscribe(ch)
	reg.Unsubscribe(ch)

	target := all[0].Name
	if reg.Active() != nil && reg.Active().Name == target {
		target = all[1].Name
	}

	if err := reg.SetActive(target); err != nil {
		t.Fatalf("SetActive(%q): %v", target, err)
	}

	select {
	case got := <-ch:
		t.Fatalf("unsubscribed channel received unexpected message: %q", got)
	case <-time.After(50 * time.Millisecond):
		// Expected: no notification.
	}
}

// TestNonBlockingSlowSubscriber verifies that a slow subscriber (full channel)
// does not block SetActive.
func TestNonBlockingSlowSubscriber(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	all := reg.All()
	if len(all) < 2 {
		t.Skip("need at least 2 themes to test non-blocking send")
	}

	// Use an unbuffered channel that nobody reads — SetActive must not block.
	ch := make(chan string) // unbuffered, always "full"
	reg.Subscribe(ch)

	target := all[0].Name
	if reg.Active() != nil && reg.Active().Name == target {
		target = all[1].Name
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = reg.SetActive(target)
	}()

	select {
	case <-done:
		// Good: SetActive returned without blocking.
	case <-time.After(time.Second):
		t.Fatal("SetActive blocked on a slow subscriber")
	}
}

// TestSafeSubscribeBuffered verifies that SafeSubscribe returns a 1-buffered
// channel that is subscribed to the registry.
func TestSafeSubscribeBuffered(t *testing.T) {
	reg, err := themes.NewRegistry("")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	all := reg.All()
	if len(all) < 2 {
		t.Skip("need at least 2 themes to test SafeSubscribe")
	}

	ch := reg.SafeSubscribe()

	// Verify it is 1-buffered.
	if cap(ch) != 1 {
		t.Fatalf("SafeSubscribe channel capacity: got %d, want 1", cap(ch))
	}

	target := all[0].Name
	if reg.Active() != nil && reg.Active().Name == target {
		target = all[1].Name
	}

	if err := reg.SetActive(target); err != nil {
		t.Fatalf("SetActive(%q): %v", target, err)
	}

	select {
	case got := <-ch:
		if got != target {
			t.Fatalf("SafeSubscribe channel got %q, want %q", got, target)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SafeSubscribe notification")
	}
}
