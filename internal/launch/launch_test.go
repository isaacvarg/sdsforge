package launch

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
)

// stubDir puts an executable stub on PATH for each name and returns the
// directory holding them.
//
// PATH is replaced rather than prepended, so nothing the developer happens to
// have installed can satisfy a lookup the test meant to fail. Same approach as
// internal/generation/browser_test.go.
func stubDir(t *testing.T, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// clearEnv removes the variables the resolvers consult, so a test only sees
// what it sets itself.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"VISUAL", "EDITOR", "SHELL"} {
		t.Setenv(name, "")
	}
}

func TestEditorPrefersConfigOverEnvironment(t *testing.T) {
	dir := stubDir(t, "configured", "visual", "editor")
	clearEnv(t)
	t.Setenv("VISUAL", "visual")
	t.Setenv("EDITOR", "editor")

	editor, err := Editor(config.Edit{Command: "configured"})
	if err != nil {
		t.Fatalf("Editor() error = %v", err)
	}
	if want := filepath.Join(dir, "configured"); editor.Path != want {
		t.Errorf("Path = %v, want %v", editor.Path, want)
	}
	if editor.Origin != "edit.command" {
		t.Errorf("Origin = %v, want edit.command", editor.Origin)
	}
}

func TestEditorPrefersVisualOverEditor(t *testing.T) {
	dir := stubDir(t, "visual", "editor")
	clearEnv(t)
	t.Setenv("VISUAL", "visual")
	t.Setenv("EDITOR", "editor")

	editor, err := Editor(config.Edit{})
	if err != nil {
		t.Fatalf("Editor() error = %v", err)
	}
	if want := filepath.Join(dir, "visual"); editor.Path != want {
		t.Errorf("Path = %v, want %v", editor.Path, want)
	}
	if editor.Origin != "$VISUAL" {
		t.Errorf("Origin = %v, want $VISUAL", editor.Origin)
	}
}

func TestEditorFallsBackToEditor(t *testing.T) {
	dir := stubDir(t, "editor")
	clearEnv(t)
	t.Setenv("EDITOR", "editor")

	editor, err := Editor(config.Edit{})
	if err != nil {
		t.Fatalf("Editor() error = %v", err)
	}
	if want := filepath.Join(dir, "editor"); editor.Path != want {
		t.Errorf("Path = %v, want %v", editor.Path, want)
	}
	if editor.Origin != "$EDITOR" {
		t.Errorf("Origin = %v, want $EDITOR", editor.Origin)
	}
}

// The built-in default is the reason 'document edit' works on a machine where
// nothing has been configured, so it must resolve rather than error.
func TestEditorFallsBackToBuiltInDefault(t *testing.T) {
	dir := stubDir(t, defaultEditor)
	clearEnv(t)

	editor, err := Editor(config.Edit{})
	if err != nil {
		t.Fatalf("Editor() error = %v", err)
	}
	if want := filepath.Join(dir, defaultEditor); editor.Path != want {
		t.Errorf("Path = %v, want %v", editor.Path, want)
	}
	if editor.Origin != "built-in default" {
		t.Errorf("Origin = %v, want built-in default", editor.Origin)
	}
}

// edit.args describe how you want your editor invoked, not which editor it is,
// so they survive the command coming from the environment.
func TestEditorAppliesConfiguredArgsToEnvironmentCommand(t *testing.T) {
	stubDir(t, "editor")
	clearEnv(t)
	t.Setenv("EDITOR", "editor")

	editor, err := Editor(config.Edit{Args: []string{"-c", "set ft=yaml"}})
	if err != nil {
		t.Fatalf("Editor() error = %v", err)
	}
	if got, want := strings.Join(editor.Args, "|"), "-c|set ft=yaml"; got != want {
		t.Errorf("Args = %v, want %v", got, want)
	}
}

func TestEditorReportsWhichSettingIsWrong(t *testing.T) {
	stubDir(t)
	clearEnv(t)

	_, err := Editor(config.Edit{Command: "not-installed"})
	if err == nil {
		t.Fatal("Editor() error = nil, want an error")
	}
	// The message has to name the setting to change; "executable file not
	// found" alone leaves the user hunting.
	if !strings.Contains(err.Error(), "edit.command") || !strings.Contains(err.Error(), "not-installed") {
		t.Errorf("error = %q, want it to name edit.command and the command", err)
	}
}

func TestShellResolution(t *testing.T) {
	dir := stubDir(t, "configured", "loginshell")

	// defaultShell is an absolute path on Unix, so it resolves to itself rather
	// than to anything in the stub directory.
	wantDefault := defaultShell
	if !filepath.IsAbs(defaultShell) {
		wantDefault = filepath.Join(dir, defaultShell)
	}

	tests := []struct {
		name       string
		cfg        config.CD
		shellEnv   string
		wantPath   string
		wantOrigin string
	}{
		{"config wins", config.CD{Command: "configured"}, "loginshell", filepath.Join(dir, "configured"), "cd.command"},
		{"then $SHELL", config.CD{}, "loginshell", filepath.Join(dir, "loginshell"), "$SHELL"},
		{"then the default", config.CD{}, "", wantDefault, "built-in default"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("SHELL", test.shellEnv)

			shell, err := Shell(test.cfg)
			if err != nil {
				t.Fatalf("Shell() error = %v", err)
			}
			if shell.Path != test.wantPath {
				t.Errorf("Path = %v, want %v", shell.Path, test.wantPath)
			}
			if shell.Origin != test.wantOrigin {
				t.Errorf("Origin = %v, want %v", shell.Origin, test.wantOrigin)
			}
		})
	}
}

func TestParseCommandSplitsArguments(t *testing.T) {
	dir := stubDir(t, "code")

	path, args, err := parseCommand("code --wait", []string{"doc.yaml"})
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if want := filepath.Join(dir, "code"); path != want {
		t.Errorf("path = %v, want %v", path, want)
	}
	// The command's own arguments come first, then the caller's file.
	if got, want := strings.Join(args, "|"), "--wait|doc.yaml"; got != want {
		t.Errorf("args = %v, want %v", got, want)
	}
}

// An editor installed somewhere with a space in the path must not be mistaken
// for a command with an argument, which is why the whole string is looked up
// before any splitting.
func TestParseCommandKeepsAPathContainingSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stubs are shell scripts")
	}

	dir := stubDir(t)
	spaced := filepath.Join(dir, "My Editor")
	if err := os.WriteFile(spaced, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing %s: %v", spaced, err)
	}

	path, args, err := parseCommand(spaced, nil)
	if err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	if path != spaced {
		t.Errorf("path = %v, want %v", path, spaced)
	}
	if len(args) != 0 {
		t.Errorf("args = %v, want none", args)
	}
}

func TestParseCommandRejectsAnEmptyCommand(t *testing.T) {
	for _, command := range []string{"", "   "} {
		if _, _, err := parseCommand(command, nil); !errors.Is(err, ErrNoCommand) {
			t.Errorf("parseCommand(%q) error = %v, want ErrNoCommand", command, err)
		}
	}
}

func TestParseCommandLeavesTheCallersArgsAlone(t *testing.T) {
	stubDir(t, "code")

	caller := []string{"doc.yaml"}
	if _, _, err := parseCommand("code --wait", caller); err != nil {
		t.Fatalf("parseCommand() error = %v", err)
	}
	// A careless append onto the split fields could write through into the
	// caller's slice.
	if got, want := strings.Join(caller, "|"), "doc.yaml"; got != want {
		t.Errorf("caller args = %v, want %v", got, want)
	}
}

func TestRunSetsTheWorkingDirectoryAndPassesArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "record")
	stub := filepath.Join(bin, "record")
	script := "#!/bin/sh\n{ pwd; echo \"$@\"; } > " + record + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("writing %s: %v", stub, err)
	}

	workDir := t.TempDir()
	program := Program{Path: stub, Args: []string{"--flag"}}
	if err := program.With("doc.yaml").Run(workDir); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("reading %s: %v", record, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("stub recorded %q, want two lines", raw)
	}
	// The temp dir may be reached through a symlink (/var -> /private/var on
	// macOS), so compare what the shell resolved rather than the raw path.
	if resolved, err := filepath.EvalSymlinks(workDir); err == nil {
		workDir = resolved
	}
	if lines[0] != workDir {
		t.Errorf("working directory = %v, want %v", lines[0], workDir)
	}
	if lines[1] != "--flag doc.yaml" {
		t.Errorf("arguments = %v, want --flag doc.yaml", lines[1])
	}
}

func TestRunPassesTheEnvironmentThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "record")
	stub := filepath.Join(bin, "env-stub")
	script := "#!/bin/sh\necho \"$SDSFORGE_SUBSHELL\" > " + record + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("writing %s: %v", stub, err)
	}

	program := Program{Path: stub}
	if err := program.Run(t.TempDir(), "SDSFORGE_SUBSHELL=1"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("reading %s: %v", record, err)
	}
	if got := strings.TrimSpace(string(raw)); got != "1" {
		t.Errorf("SDSFORGE_SUBSHELL = %q, want 1", got)
	}
}

// 'cd' passes a subshell's status through instead of dressing it up as an
// sdsforge failure, which depends on the exit status surviving Run's wrapping.
func TestExitCodeSurvivesRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	stub := filepath.Join(t.TempDir(), "failing")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("writing %s: %v", stub, err)
	}

	err := Program{Path: stub}.Run(t.TempDir())
	if err == nil {
		t.Fatal("Run() error = nil, want a non-zero exit")
	}
	code, ok := ExitCode(err)
	if !ok {
		t.Fatalf("ExitCode(%v) ok = false, want true", err)
	}
	if code != 3 {
		t.Errorf("ExitCode = %d, want 3", code)
	}
}

// A program that could not be started is a real failure to report, not a status
// to pass through.
func TestExitCodeRejectsAFailureToStart(t *testing.T) {
	err := Program{Path: filepath.Join(t.TempDir(), "missing")}.Run("")
	if err == nil {
		t.Fatal("Run() error = nil, want an error")
	}
	if _, ok := ExitCode(err); ok {
		t.Errorf("ExitCode(%v) ok = true, want false", err)
	}
}

func TestArgsStringQuotesOnlyWhatNeedsIt(t *testing.T) {
	program := Program{Path: "/usr/bin/nvim", Args: []string{"-c", "set ft=yaml"}}
	if got, want := program.ArgsString(), `-c "set ft=yaml"`; got != want {
		t.Errorf("ArgsString() = %v, want %v", got, want)
	}
	if got, want := program.String(), `/usr/bin/nvim -c "set ft=yaml"`; got != want {
		t.Errorf("String() = %v, want %v", got, want)
	}
}
