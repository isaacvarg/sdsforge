package generation

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNoBrowser reports that no Chrome-based browser could be located.
//
// A sentinel so a caller can tell "you have no browser installed" apart from
// "the browser failed": the first is a setup problem with a fixed remedy, the
// second is not.
var ErrNoBrowser = errors.New("no Chrome-based browser found")

// browserNames are searched on PATH, in order.
//
// Chromium and Chrome first because they are the reference implementations of
// the DevTools protocol this uses; the Chromium forks follow. Anything not on
// this list is still reachable through the 'browser' config key.
var browserNames = []string{
	"chromium",
	"chromium-browser",
	"google-chrome",
	"google-chrome-stable",
	"brave",
	"brave-browser",
	"microsoft-edge",
	"microsoft-edge-stable",
}

// FindBrowser resolves the browser to print with.
//
// A configured value is resolved ALONE: if the user named a browser, falling
// back to whatever else is installed would print with something they did not
// ask for and never say so.
//
// chromedp does its own search, but it is silent about what it tried. Owning
// the lookup is what makes the error below possible, and what lets
// 'config show' report the browser without printing anything.
func FindBrowser(configured string) (string, error) {
	if configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf(
				"pdf.browser is set to %q, which could not be run: %w", configured, err)
		}
		return path, nil
	}

	for _, name := range browserNames {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf(`%w.

Searched PATH for: %s

Install one, or name yours in the config file:

  [pdf]
  browser = "/path/to/your/browser"`,
		ErrNoBrowser, strings.Join(browserNames, ", "))
}
