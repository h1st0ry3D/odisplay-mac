// Package cli talks to the odisplay program.
//
// This is the same contract the Omarchy panel uses: `list --json` to read, and
// `switch` and `set` to change. Nothing here decides the order a switch happens
// in, because that belongs to odisplay and is the one thing that must not exist
// twice.
//
// Everything in this package runs on any platform, so it is all testable
// without a Mac. The menu bar around it is not.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Candidate directories, in order. The first is the conventional place for
// something installed system-wide, the second needs no privileges, and they
// match what the Omarchy panel looks in so one install serves both.
//
// These are directories, not binary names: the name is appended. An empty entry
// means the user's own, which is read from the environment rather than guessed,
// so this works the same under a login shell, a launch agent and a terminal.
var candidates = []string{
	"/usr/local/bin",
	"",
	"/opt/homebrew/bin",
}

// ErrMissing reports that the odisplay program is not installed, which is a
// different situation from odisplay reporting that something else is missing.
var ErrMissing = errors.New("odisplay is not installed")

// Timeout is the longest a single call may take.
//
// It has to clear the slowest odisplay is allowed to be: eight seconds for the
// display and ten for each device, with the mouse last. It is deliberately
// longer than that, because this is a menu and a person waiting on it would
// rather it finished than that it was prompt.
const Timeout = 45 * time.Second

// Input is one monitor input source.
type Input struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	VCP  int    `json:"vcp"`
	// Host is the Easy-Switch channel, "0" through "3". odisplay leaves the
	// key out of the file when it is off, so an absent value means off.
	Host string `json:"host"`
	// Hidden keeps this input out of the menu without removing it. The Hide
	// submenu is the one place it still appears, so it can be brought back.
	Hidden bool `json:"hidden"`
	// Power is what this Mac does with itself after a switch sends the keyboard
	// and mouse away. Empty means nothing, which is the default.
	Power string `json:"power"`
}

// Host returns the channel, with an absent or unusable value read as off.
//
// Only 0 to 3 are real channels. Anything else is refused rather than passed
// on, so a settings file edited by hand cannot put a number in front of odisplay
// that no channel has.
func (i Input) HostChannel() string {
	switch i.Host {
	case "1", "2", "3":
		return i.Host
	default:
		return "0"
	}
}

// Label is the name to show, falling back to the key so a row is never blank.
func (i Input) Label() string {
	if strings.TrimSpace(i.Name) != "" {
		return i.Name
	}
	return i.Key
}

// MonitorOr is the program to name in a label, falling back to a neutral word
// when odisplay did not say. A label that names nothing is worse than one that
// names nothing in particular.
func (s Settings) MonitorOr() string {
	if s.Monitor != "" {
		return s.Monitor
	}
	return "display"
}

// Visible is the inputs the menu offers. A hidden one is left out here and in
// the Hide submenu alone, which is where it can be brought back.
func (s Settings) Visible() []Input {
	out := make([]Input, 0, len(s.Inputs))
	for _, in := range s.Inputs {
		if !in.Hidden {
			out = append(out, in)
		}
	}
	return out
}

// Settings is what `list --json` prints.
type Settings struct {
	Version int `json:"version"`
	// Display is the number in force on the machine asking, which odisplay
	// has already resolved from its per-platform entries. There is nothing to
	// do with platforms here, which is the point.
	Display  int            `json:"display"`
	Displays map[string]int `json:"displays"`
	// Monitor is the program that moves the display on this machine, named by
	// odisplay so a label says which one is actually being asked. It is not the
	// same on every Mac, and naming the wrong one is misleading rather than
	// merely untidy.
	Monitor string   `json:"monitor"`
	Inputs  []Input  `json:"inputs"`
	Devices []string `json:"devices"`
}

// Client is a handle on the odisplay program.
type Client struct {
	// Path is the binary to run, once found.
	Path string
}

// New finds odisplay and returns a client for it, or ErrMissing.
func New() (*Client, error) {
	for _, p := range Candidates() {
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return &Client{Path: p}, nil
		}
	}
	return nil, fmt.Errorf("%w: looked in %s", ErrMissing, strings.Join(Candidates(), ", "))
}

// Candidates is every path New will look, in order, for a message that says
// where it looked.
func Candidates() []string {
	home := os.Getenv("HOME")
	out := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if dir == "" {
			dir = filepath.Join(home, ".local", "bin")
		}
		out = append(out, filepath.Join(dir, "odisplay"))
	}
	return out
}

// Result is what one odisplay call said.
type Result struct {
	// Code is the exit code, which is the contract.
	Code int
	// Message is the one line odisplay wrote for a person.
	Message string
	// Detail is what it wrote underneath, usually the reason from the program
	// that failed.
	Detail string
}

// Outcome is the result read as one of the situations the exit codes name. The
// codes are kept apart rather than collapsed into pass and fail, because two of
// them mean opposite things about where the user's keyboard is.
type Outcome int

const (
	// OutcomeOK: everything asked for happened.
	OutcomeOK Outcome = iota
	// OutcomeUsage: the settings or the request was wrong. Nothing moved.
	OutcomeUsage
	// OutcomeDisplayFailed: the monitor did not move, so the devices were
	// deliberately left alone. The keyboard is still here.
	OutcomeDisplayFailed
	// OutcomeDevicesFailed: the monitor moved and the devices did not. The
	// keyboard is on another machine and only the button underneath can bring
	// it back.
	OutcomeDevicesFailed
	// OutcomeMissing: a program this needs is not installed. Which one decides
	// whether the keyboard is here or not.
	OutcomeMissing
)

func (r Result) Outcome() Outcome {
	switch r.Code {
	case 0:
		return OutcomeOK
	case 2:
		return OutcomeDisplayFailed
	case 3:
		return OutcomeDevicesFailed
	case 4:
		return OutcomeMissing
	default:
		return OutcomeUsage
	}
}

// Sent reports whether a switch was completed, which is not the same as the
// program having exited without complaint.
func (r Result) Sent() bool { return r.Outcome() == OutcomeOK }

// MessageOr returns the sentence to show, falling back to one written here when
// odisplay said nothing. The fallback is what a person reads at the moment they
// most need to know where their keyboard is, so it has to say that rather than
// repeat an exit code.
func (r Result) MessageOr(inputLabel string) string {
	if r.Message != "" {
		return r.Message
	}
	switch r.Outcome() {
	case OutcomeOK:
		return "Sent " + inputLabel + "."
	case OutcomeUsage:
		return "Could not read the odisplay settings."
	case OutcomeDisplayFailed:
		return "Could not switch. The keyboard and mouse were left alone."
	case OutcomeDevicesFailed:
		return "The display moved but the keyboard and mouse did not. The switch underneath is the way back."
	case OutcomeMissing:
		return "A program this needs is not installed."
	}
	return "odisplay exited with code " + fmt.Sprint(r.Code) + "."
}

// DetailOr returns the reason, with the same reasoning about the fallback.
func (r Result) DetailOr() string {
	if r.Detail != "" {
		return r.Detail
	}
	switch r.Outcome() {
	case OutcomeDisplayFailed:
		return "The monitor did not change, so nothing was sent to the keyboard and mouse."
	case OutcomeDevicesFailed:
		return "They may be on another machine. Press the channel button underneath one."
	}
	return ""
}

// run calls odisplay and reads back both of its answers.
//
// The program and its arguments go across as an argv array with no shell
// between, so a device name with spaces in it stays one argument and a name
// with a quote in it cannot become a second command. The child gets a PATH of
// its own, so nothing in this session's environment can change what a program
// name resolves to inside odisplay.
func (c *Client) run(ctx context.Context, args ...string) (Result, error) {
	if c == nil || c.Path == "" {
		return Result{}, ErrMissing
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Path, args...)
	// A fixed environment, so nothing in this session can change what a program
	// name resolves to inside odisplay. The two it needs to find its own
	// settings are passed through, because a launch agent starts with a bare
	// environment and would otherwise find no config at all.
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"LANG=C",
		"LC_ALL=C",
		"HOME=" + os.Getenv("HOME"),
		"XDG_CONFIG_HOME=" + os.Getenv("XDG_CONFIG_HOME"),
		// Forwarded, not just supported: without it every call reads the
		// real settings file no matter what this process was told to use.
		"ODISPLAY_CONFIG=" + os.Getenv("ODISPLAY_CONFIG"),
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := Result{
		Message: flatten(out.String()),
		Detail:  flatten(errBuf.String()),
	}
	if cmd.ProcessState != nil {
		res.Code = cmd.ProcessState.ExitCode()
	}
	if err != nil && res.Code == 0 {
		// Killed by a signal, or never started. ExitCode reports -1 there,
		// which is not an exit code odisplay ever uses.
		res.Code = -1
	}
	return res, nil
}

// List reads the settings. A non-zero exit is an error rather than a Result,
// because there is no useful fallback and the caller has to show something.
func (c *Client) List(ctx context.Context) (Settings, error) {
	res, err := c.run(ctx, "list", "--json")
	if err != nil {
		return Settings{}, err
	}
	if res.Code != 0 {
		return Settings{}, fmt.Errorf("odisplay could not read its settings (exit %d): %s",
			res.Code, firstNonEmpty(res.Detail, res.Message))
	}
	var s Settings
	if err := json.Unmarshal([]byte(res.Message), &s); err != nil {
		return Settings{}, fmt.Errorf("odisplay printed settings this does not understand: %w", err)
	}
	if len(s.Inputs) == 0 {
		return s, nil
	}
	return s, nil
}

// Switch asks for a named input.
func (c *Client) Switch(ctx context.Context, key string) Result {
	res, _ := c.run(ctx, "switch", key)
	return res
}

// SetHost sets an input's Easy-Switch channel.
func (c *Client) SetHost(ctx context.Context, key, host string) Result {
	res, _ := c.run(ctx, "set", "host", key, host)
	return res
}

// SetDisplay records which display this machine should switch, by the number
// the monitor program uses here.
//
// Only this machine's entry is written: odisplay keeps one number per platform,
// so setting it on a Mac cannot change what a Linux box uses.
func (c *Client) SetDisplay(ctx context.Context, number int) Result {
	res, _ := c.run(ctx, "set", "display", strconv.Itoa(number))
	return res
}

// SetHidden shows or hides an input in the menu. It switches nothing, so it is
// safe to do to the input that is in use.
func (c *Client) SetHidden(ctx context.Context, key string, hidden bool) Result {
	word := "off"
	if hidden {
		word = "on"
	}
	res, _ := c.run(ctx, "set", "hidden", key, word)
	return res
}

// SetPower records what this Mac does with itself after switching away.
func (c *Client) SetPower(ctx context.Context, key, action string) Result {
	if action == "" {
		action = "none"
	}
	res, _ := c.run(ctx, "set", "power", key, action)
	return res
}

// SetName renames an input.
func (c *Client) SetName(ctx context.Context, key, name string) Result {
	res, _ := c.run(ctx, "set", "name", key, name)
	return res
}

// Doctor asks whether this machine is ready. It touches no hardware.
func (c *Client) Doctor(ctx context.Context) (string, Result) {
	res, _ := c.run(ctx, "doctor")
	return res.Message + res.Detail, res
}

// flatten makes odisplay's output one line, because a menu item is one line.
func flatten(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
