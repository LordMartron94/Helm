package cli

import "testing"

func TestHelmQuietModeEnabled(t *testing.T) {
	t.Setenv("HELM_QUIET", "1")
	if !helmQuietModeEnabled() {
		t.Fatal("expected HELM_QUIET=1 to enable quiet mode")
	}

	t.Setenv("HELM_QUIET", "true")
	if !helmQuietModeEnabled() {
		t.Fatal("expected HELM_QUIET=true to enable quiet mode")
	}

	t.Setenv("HELM_QUIET", "")
	if helmQuietModeEnabled() {
		t.Fatal("expected empty HELM_QUIET to disable quiet mode")
	}
}

func TestShouldSuppressStartupBannerWhenHelmQuiet(t *testing.T) {
	t.Setenv("HELM_QUIET", "1")
	if !shouldSuppressStartupBanner(nil, []string{"export-graph", "entry"}) {
		t.Fatal("expected HELM_QUIET to suppress startup banner")
	}
}
