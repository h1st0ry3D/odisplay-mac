// Package dialog asks the person using this a question.
//
// macOS has no way to prompt for text from a menu bar item without a window of
// your own, so this goes through osascript, which is part of the system. That
// gives a real macOS dialog rather than one this program draws: it is keyboard
// navigable, it follows the system theme, and it is what people already expect
// a menu bar app to do.
//
// The AppleScript is built here rather than assembled at the call site, and it
// is the part worth testing. Everything else is one exec of a system binary.
package dialog

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Timeout is how long a dialog may stay open. Long, because it is waiting on a
// person rather than on a machine, and there is no reason to close a dialog
// someone is still typing in.
const Timeout = 5 * time.Minute

// osascript is part of macOS and is always at this path.
const osascript = "/usr/bin/osascript"

// ErrCancelled is returned when the dialog was dismissed rather than answered.
// It is not a failure: changing your mind is a normal thing to do with a
// prompt, and the caller should say nothing about it.
var ErrCancelled = errors.New("dialog cancelled")

// Script is the AppleScript to run, as a single -e argument.
//
// Exported so the quoting can be checked on a machine that is not a Mac. The
// values come from the settings file, which is hand-editable, so a name
// containing a quote is a name this has to survive rather than one it can
// assume away.
func Script(prompt, defaultValue string) string {
	return fmt.Sprintf(
		`text returned of (display dialog %s default answer %s `+
			`with title "Odisplay" buttons {"Cancel", "Save"} default button "Save"`,
		literal(prompt), literal(defaultValue))
}

// literal makes a Go string into an AppleScript string literal.
//
// AppleScript escapes a backslash and a double quote with a backslash, and a
// literal string cannot span lines, so anything that would break the line is
// dropped rather than escaped. That matches what the settings file keeps
// anyway: a name is one line of one menu item.
func literal(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n' || r == '\r' || r == '\t':
			// A literal cannot span a line. A space keeps the words apart
			// instead of running them together.
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// Other control characters cannot appear in a literal at all.
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Ask shows a text prompt and returns what was typed.
//
// ok is false when the dialog was dismissed. err is for anything else, which is
// worth showing: a dialog that cannot open is a feature that does not work, and
// saying nothing would leave it looking like one that does.
func Ask(ctx context.Context, prompt, defaultValue string) (text string, ok bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascript, "-e", Script(prompt, defaultValue))
	// A bare environment. osascript needs no more, and nothing in the session
	// should be able to change what it does.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}

	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		said := errBuf.String()
		if isCancelled(said) {
			return "", false, ErrCancelled
		}
		if said == "" {
			said = err.Error()
		}
		return "", false, fmt.Errorf("osascript: %s", strings.TrimSpace(said))
	}

	// osascript appends a newline to what it prints.
	return strings.TrimRight(out.String(), "\r\n"), true, nil
}

// isCancelled reports whether osascript failed because the dialog was dismissed.
//
// Dismissing one exits non-zero like any other failure, so the two have to be
// told apart by what it said. -128 is AppleScript's "user cancelled" code.
func isCancelled(said string) bool {
	return strings.Contains(said, "(-128)") ||
		strings.Contains(strings.ToLower(said), "user canceled") ||
		strings.Contains(strings.ToLower(said), "user cancelled")
}
