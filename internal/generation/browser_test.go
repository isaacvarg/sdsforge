package generation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubBrowser creates an executable file named name in dir.
func stubBrowser(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestFindBrowserSearchesPATH(t *testing.T) {
	dir := t.TempDir()
	want := stubBrowser(t, dir, "google-chrome")
	t.Setenv("PATH", dir)

	got, err := FindBrowser("")
	if err != nil {
		t.Fatalf("FindBrowser() error = %v", err)
	}
	if got != want {
		t.Errorf("FindBrowser() = %q, want %q", got, want)
	}
}

// The order in browserNames is a preference, not an accident: the reference
// implementations come before the forks.
func TestFindBrowserPrefersEarlierNames(t *testing.T) {
	dir := t.TempDir()
	want := stubBrowser(t, dir, "chromium")
	stubBrowser(t, dir, "brave")
	stubBrowser(t, dir, "google-chrome")
	t.Setenv("PATH", dir)

	got, err := FindBrowser("")
	if err != nil {
		t.Fatalf("FindBrowser() error = %v", err)
	}
	if got != want {
		t.Errorf("FindBrowser() = %q, want %q", got, want)
	}
}

func TestFindBrowserConfigured(t *testing.T) {
	dir := t.TempDir()
	absolute := stubBrowser(t, dir, "my-browser")
	stubBrowser(t, dir, "chromium")
	t.Setenv("PATH", dir)

	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{"absolute path", absolute, absolute},
		{"bare name on PATH", "my-browser", absolute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindBrowser(tt.configured)
			if err != nil {
				t.Fatalf("FindBrowser(%q) error = %v", tt.configured, err)
			}
			if got != tt.want {
				t.Errorf("FindBrowser(%q) = %q, want %q", tt.configured, got, tt.want)
			}
		})
	}
}

// A user who named a browser meant that browser. Falling back would print with
// something they did not ask for and never say so.
func TestFindBrowserConfiguredNeverFallsBack(t *testing.T) {
	dir := t.TempDir()
	stubBrowser(t, dir, "chromium")
	t.Setenv("PATH", dir)

	_, err := FindBrowser("/nonexistent/browser")
	if err == nil {
		t.Fatal("FindBrowser() with a missing configured browser succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "/nonexistent/browser") {
		t.Errorf("error does not name the configured browser: %v", err)
	}
	if !strings.Contains(err.Error(), "pdf.browser") {
		t.Errorf("error does not name the config key: %v", err)
	}
	if errors.Is(err, ErrNoBrowser) {
		t.Error("a configured browser that is missing should not report ErrNoBrowser")
	}
}

func TestFindBrowserNoneInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := FindBrowser("")
	if err == nil {
		t.Fatal("FindBrowser() with an empty PATH succeeded, want an error")
	}
	if !errors.Is(err, ErrNoBrowser) {
		t.Errorf("error is not ErrNoBrowser: %v", err)
	}

	// The message has to say what was tried, or the user cannot tell whether
	// their browser was simply not on the list.
	for _, name := range browserNames {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not list %q: %v", name, err)
		}
	}
	if !strings.Contains(err.Error(), "[pdf]") {
		t.Errorf("error does not show how to configure one: %v", err)
	}
}
