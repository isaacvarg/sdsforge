//go:build windows

package launch

// The last resort when neither the config file nor the environment names one.
//
// notepad and cmd.exe ship with Windows, so neither can be missing. $EDITOR and
// $SHELL are rarely set there, which makes these the usual case rather than the
// fallback they are on Unix.
const (
	defaultEditor = "notepad"
	defaultShell  = "cmd.exe"
)
