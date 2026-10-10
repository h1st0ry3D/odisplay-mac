// Package panel turns settings into the shape of the menu.
//
// It is deliberately free of any drawing. What a row says, which item is
// ticked, what clicking one would do, and where the separators fall are all
// decided here and can be checked without a menu bar, a screen or a Mac. The
// systray code does nothing but put these on screen.
//
// The layout is the same shape as the Omarchy panel's: one block per input, the
// input's own name on the first item, and its Easy-Switch channel on a submenu
// under it. What differs is that a native menu cannot draw two columns, so the
// two halves of a row are two items rather than two buttons side by side.
package panel

import (
	"fmt"
	"strings"

	"github.com/h1st0ry3D/odisplay-bar/internal/cli"
)

// Action is what an item does when it is clicked.
type Action int

const (
	// ActionNone is a label, a heading, or a separator.
	ActionNone Action = iota
	// ActionSwitch moves the monitor and, if the input has a channel, the
	// devices with it. This is one call and odisplay owns the order.
	ActionSwitch
	// ActionSetHost records a channel. It only writes the setting; nothing
	// moves until that input is switched to.
	ActionSetHost
	// ActionReload re-reads the settings, for after a hand edit.
	ActionReload
	// ActionQuit leaves.
	ActionQuit
)

// Item is one thing in the menu.
type Item struct {
	// Title is the text.
	Title string
	// Tooltip is the longer explanation. A menu has room for it where a bar
	// button does not.
	Tooltip string
	// Checked draws a tick.
	Checked bool
	// Disabled greys it out and makes it do nothing. Headings and the last
	// result line are like this.
	Disabled bool
	// SeparatorBefore draws a line above. Used between inputs rather than
	// between every item, which would be a wall.
	SeparatorBefore bool
	// Children make this a submenu. Empty for an ordinary item.
	Children []*Item
	// Action is what clicking does. Ignored when Disabled.
	Action Action
	// Key is the input this belongs to, for an action that needs one.
	Key string
	// Host is the channel an ActionSetHost will write.
	Host string
}

// Status is the line at the top, which reports the last switch.
//
// It is one line because a menu item is one line, and it says where the
// keyboard is rather than repeating an exit code, because that is the only
// thing a person opening the menu after a bad switch actually needs.
type Status struct {
	Text  string
	Error bool
}

// Hosts are the Easy-Switch channels a menu offers. "0" is off, which is a real
// choice and not the absence of one, so it is listed first like any other.
var Hosts = []string{"0", "1", "2", "3"}

// HostLabel is what a channel reads as in a menu.
func HostLabel(host string) string {
	if host == "0" || host == "" {
		return "off"
	}
	return host
}

// HostMeaning is the longer sentence, which is what the submenu item carries.
func HostMeaning(host string) string {
	if host == "0" || host == "" {
		return "Keyboard and mouse stay here"
	}
	return "Move keyboard and mouse to host " + host
}

// Model is the whole menu.
type Model struct {
	Status Status
	Items  []*Item
}

// Idle is the status before anything has happened in this session.
var Idle = Status{Text: "Set input source"}

// ForSwitch builds the line that reports a finished switch.
//
// ok names the outcome so the two cases that matter can be told apart in a
// glance: the devices moved with the monitor, or they did not and the button
// underneath is the way back.
func ForSwitch(res cli.Result, label string) Status {
	if res.Outcome() == cli.OutcomeOK {
		return Status{Text: oneLine(res.MessageOr(label))}
	}
	return Status{Text: oneLine(res.MessageOr(label)), Error: true}
}

// ForSetting builds the line that reports a saved change, which is the same
// shape as a failed switch: a change that did not take must not look like one
// that did.
func ForSetting(res cli.Result, what string) Status {
	if res.Outcome() == cli.OutcomeOK {
		return Status{Text: oneLine(what)}
	}
	return Status{Text: oneLine(res.MessageOr(what)), Error: true}
}

// oneLine makes a string fit a menu item.
//
// A menu item is one line by definition, and text that does not fit gets its
// middle replaced by an ellipsis, which would cut out the part explaining what
// happened. Runs of whitespace become one space, so a wrapped reason reads as
// the sentence it is rather than as one run-together word.
//
// The client already flattens, so this is belt and braces: a Result can be
// built anywhere, including by a future caller, and the panel's own promise
// about one line should not depend on all of them remembering.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// NotInstalled is the whole menu when there is nothing to drive.
//
// It says where it looked, because "nothing happens when I click" is the worst
// thing a menu bar app can do and this is the one case that causes it.
func NotInstalled(where []string) *Model {
	m := &Model{
		Status: Status{
			Text:  "odisplay is not installed.",
			Error: true,
		},
		Items: []*Item{{
			Title:    "Put the odisplay binary in one of these:",
			Disabled: true,
		}},
	}
	// The paths are listed rather than summarized. "It does nothing" with no
	// place to put the file is the least useful thing a menu bar app can do,
	// and this is the one state where that is the whole story.
	for _, p := range where {
		m.Items = append(m.Items, &Item{
			Title:           p,
			Disabled:        true,
			SeparatorBefore: len(m.Items) == 1,
		})
	}
	m.Items = append(m.Items, &Item{
		Title:           "Reload settings",
		Tooltip:         "Look again, after putting it there",
		Action:          ActionReload,
		SeparatorBefore: true,
	})
	return m
}

// Build turns settings into the menu.
//
// busy marks a switch in flight. Every action is refused while it is set, so
// two switches cannot interleave, which on a keyboard and mouse means one
// machine with a mouse and the other without one.
func Build(s cli.Settings, status Status, busy bool) *Model {
	m := &Model{Status: status}
	m.Items = append(m.Items, &Item{
		Title:    oneLine(status.Text),
		Tooltip:  tooltipFor(status),
		Disabled: true,
	})

	if len(s.Inputs) == 0 {
		m.Items = append(m.Items, &Item{
			Title:    "No inputs are configured.",
			Disabled: true,
		})
	}

	for _, in := range s.Inputs {
		label := in.Label()
		host := in.HostChannel()

		m.Items = append(m.Items, &Item{
			Title:           label,
			Tooltip:         "Switch the monitor to " + label + ", and the devices with it",
			SeparatorBefore: true,
			Action:          ActionSwitch,
			Key:             in.Key,
			Disabled:        busy,
		})

		hostItem := &Item{
			Title:           "Keyboard and mouse: " + HostLabel(host),
			Tooltip:         HostMeaning(host),
			Action:          ActionSetHost,
			Key:             in.Key,
			Disabled:        busy,
			SeparatorBefore: false,
		}
		for _, h := range Hosts {
			hostItem.Children = append(hostItem.Children, &Item{
				Title:   HostLabel(h),
				Tooltip: HostMeaning(h),
				// The tick is on the channel in force, which is the same rule
				// as the panel's cycle button: reading it is how you know what
				// a switch will do before you make one.
				Checked:  h == host,
				Action:   ActionSetHost,
				Key:      in.Key,
				Host:     h,
				Disabled: busy,
			})
		}
		m.Items = append(m.Items, hostItem)
	}

	m.Items = append(m.Items, &Item{
		Title:           "Reload settings",
		Tooltip:         "Re-read the settings file, for after a hand edit",
		SeparatorBefore: true,
		Action:          ActionReload,
		Disabled:        busy,
	})

	m.Items = append(m.Items, &Item{
		Title:    "Quit",
		Tooltip:  "Leave. Nothing is switched on the way out.",
		Action:   ActionQuit,
		Disabled: false,
	})

	return m
}

func tooltipFor(s Status) string {
	if s.Error {
		return "The last switch did not finish. Open the menu again after reading this."
	}
	return ""
}

// Describe renders the model as text, which is what the tests check and what a
// person reads first when something is wrong.
func (m *Model) Describe() string {
	var b strings.Builder
	mark := func(st Status) string {
		if st.Error {
			return st.Text + "  (error)"
		}
		return st.Text
	}
	b.WriteString(mark(m.Status))
	for _, it := range m.Items {
		if it.SeparatorBefore {
			b.WriteString("\n--")
		}
		flags := ""
		if it.Checked {
			flags += " [x]"
		}
		if it.Disabled {
			flags += " [disabled]"
		}
		b.WriteString("\n  " + it.Title + flags)
		for _, c := range it.Children {
			child := c.Title
			if c.Checked {
				child = "[x] " + child
			}
			b.WriteString("\n      - " + child)
		}
	}
	return b.String()
}

// Summary is a one-line count, used in the log when the menu is rebuilt, so a
// menu that came out empty says why.
func (m *Model) Summary() string {
	return fmt.Sprintf("%d items, %d inputs", len(m.Items), len(m.Items)/2)
}
