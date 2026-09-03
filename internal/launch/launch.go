// Package launch resolves and runs the external programs sdsforge hands control
// to: the user's editor and the user's shell.
//
// It exists as its own package rather than as helpers in cmd/ because the
// resolution rules are the interesting part -- which of four sources an editor
// came from, whether a command string is a path or a command with arguments --
// and they are worth testing.
package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/config"
)

// ErrNoCommand reports that nothing was named to run.
//
// A sentinel so a caller can tell "you configured an empty command" apart from
// "the command you named is not installed": the first is a mistake in the
// config file, the second a missing program.
var ErrNoCommand = errors.New("no command to run")

// Program is a resolved external command, ready to run.
type Program struct {
	// Path is the executable, already looked up on PATH.
	Path string
	// Args are passed to it, in order.
	Args []string
	// Origin says where the command came from, so 'config show' can report
	// which of the four sources won.
	Origin string
}

// String renders the whole command for a message.
func (p Program) String() string {
	return quote(append([]string{p.Path}, p.Args...))
}

// ArgsString renders just the arguments, for a caller that has already shown
// the path.
func (p Program) ArgsString() string { return quote(p.Args) }

// quote joins parts for display, quoting only those that need it so the result
// can be pasted back into a shell.
func quote(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || strings.ContainsAny(part, " \t\"'\\") {
			part = fmt.Sprintf("%q", part)
		}
		quoted = append(quoted, part)
	}
	return strings.Join(quoted, " ")
}

// With returns the program with further arguments appended.
func (p Program) With(args ...string) Program {
	if len(args) == 0 {
		return p
	}
	combined := make([]string, 0, len(p.Args)+len(args))
	combined = append(combined, p.Args...)
	combined = append(combined, args...)
	p.Args = combined
	return p
}

// Run runs the program and waits for it to finish.
//
// dir is its working directory; empty means inherit this process's. env holds
// extra "NAME=value" entries layered over the current environment.
//
// The real os.Std* files are used rather than cobra's writers, which is a
// deliberate departure from the convention everywhere else in sdsforge: an
// editor and an interactive shell both drive the terminal directly, and
// cobra's accessors return an io.Writer that need not be a *os.File.
//
// exec.Command rather than exec.CommandContext, also deliberately, and unlike
// every other subprocess in this codebase. Execute installs a
// signal.NotifyContext so a print can be interrupted; binding an interactive
// child to that context would kill the user's editor the moment they pressed
// Ctrl-C inside it. The child shares this process's foreground process group
// and is sent the signal itself.
func (p Program) Run(dir string, env ...string) error {
	cmd := exec.Command(p.Path, p.Args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", p.Path, err)
	}
	return nil
}

// ExitCode reports the status a program exited with, and whether it ran at all.
//
// ok is false when the program could not be started, which is a real failure to
// report; it is true when the program ran and chose its own status, which a
// caller may want to pass through rather than dress up as an sdsforge error.
func ExitCode(err error) (code int, ok bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// Editor resolves the editor to open a document with.
//
// The order is the one every Unix tool uses -- configuration, then $VISUAL,
// then $EDITOR -- ending at a built-in default rather than an error, so
// 'document edit' works on a fresh machine where nothing has been set.
//
// edit.args are passed whichever source the command came from: they describe
// how you want your editor invoked, not which editor it is.
func Editor(cfg config.Edit) (Program, error) {
	if cfg.Command != "" {
		return resolve(cfg.Command, cfg.Args, "edit.command")
	}
	if command := os.Getenv("VISUAL"); command != "" {
		return resolve(command, cfg.Args, "$VISUAL")
	}
	if command := os.Getenv("EDITOR"); command != "" {
		return resolve(command, cfg.Args, "$EDITOR")
	}
	return resolve(defaultEditor, cfg.Args, "built-in default")
}

// Shell resolves the shell to launch in a document's directory.
func Shell(cfg config.CD) (Program, error) {
	if cfg.Command != "" {
		return resolve(cfg.Command, cfg.Args, "cd.command")
	}
	if command := os.Getenv("SHELL"); command != "" {
		return resolve(command, cfg.Args, "$SHELL")
	}
	return resolve(defaultShell, cfg.Args, "built-in default")
}

// resolve turns one candidate command into a Program, naming where it came from
// so a failure says which setting to go and fix.
func resolve(command string, args []string, origin string) (Program, error) {
	path, parsed, err := parseCommand(command, args)
	if err != nil {
		return Program{}, fmt.Errorf("%s (%q): %w", origin, command, err)
	}
	return Program{Path: path, Args: parsed, Origin: origin}, nil
}

// parseCommand splits a command string into an executable and its arguments.
//
// The WHOLE string is looked up first, so an editor installed somewhere with a
// space in the path ("/opt/My Editor/bin/edit") is not mistaken for a command
// with an argument. Only when that fails is the string split, which is what
// makes EDITOR="code --wait" work.
//
// The split is on whitespace, so quoting inside the string is not honoured --
// unlike chezmoi, which parses these with a real shell grammar. Carrying a
// shell parser for the sake of EDITOR="nvim -c 'set ft=yaml'" is not worth it
// when the args config key expresses the same thing without any quoting:
//
//	[edit]
//	command = "nvim"
//	args    = ["-c", "set ft=yaml"]
func parseCommand(command string, args []string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, ErrNoCommand
	}

	if path, err := exec.LookPath(command); err == nil {
		return path, args, nil
	}

	fields := strings.Fields(command)
	path, err := exec.LookPath(fields[0])
	if err != nil {
		return "", nil, err
	}

	combined := make([]string, 0, len(fields)-1+len(args))
	combined = append(combined, fields[1:]...)
	combined = append(combined, args...)
	return path, combined, nil
}
