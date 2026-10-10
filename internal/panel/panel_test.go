package panel

import (
	"strings"
	"testing"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
)

func settings() cli.Settings {
	return cli.Settings{
		Display: 1,
		Inputs: []cli.Input{
			{Key: "usbc", Name: "X", VCP: 27, Host: "0"},
			{Key: "dp", Name: "Mac Mini", VCP: 15, Host: "3"},
			{Key: "hdmi", Name: "PC", VCP: 17},
		},
	}
}

// find returns the first item with the given title.
func find(m *Model, title string) *Item {
	for _, it := range m.Items {
		if it.Title == title {
			return it
		}
	}
	return nil
}

func TestEveryInputGetsItsOwnNameAndHostControl(t *testing.T) {
	m := Build(settings(), Idle, false)
	for _, want := range []string{"X", "Mac Mini", "PC"} {
		if find(m, want) == nil {
			t.Errorf("no item for input %q", want)
		}
	}
	for _, want := range []string{
		"Keyboard and mouse: off",
		"Keyboard and mouse: 3",
		"Keyboard and mouse: off",
	} {
		if find(m, want) == nil {
			t.Errorf("no host control reading %q", want)
		}
	}
}

// The user's name is what shows. An input with no name falls back to its key so
// a row is never blank.
func TestTheUsersNameIsWhatShows(t *testing.T) {
	m := Build(settings(), Idle, false)
	if find(m, "Mac Mini") == nil {
		t.Error("the custom name is not shown")
	}

	s := cli.Settings{Inputs: []cli.Input{{Key: "dp"}}}
	m2 := Build(s, Idle, false)
	if find(m2, "dp") == nil {
		t.Error("an unnamed input shows nothing")
	}
}

// The tick marks the channel a switch would actually use, which is how you know
// what is about to happen before making it happen.
func TestTheTickIsOnTheChannelInForce(t *testing.T) {
	m := Build(settings(), Idle, false)
	host := find(m, "Keyboard and mouse: 3")
	if host == nil {
		t.Fatal("no host control for Mac Mini")
	}
	if len(host.Children) != 4 {
		t.Fatalf("choices = %d, want 4", len(host.Children))
	}
	ticked := 0
	for _, c := range host.Children {
		if c.Checked {
			ticked++
			if c.Host != "3" {
				t.Errorf("ticked %q, want the channel in force (3)", c.Host)
			}
		}
	}
	if ticked != 1 {
		t.Errorf("ticked %d choices, want exactly 1", ticked)
	}
}

// Every channel is offered, off included. Off is a real choice, not the
// absence of one, so it is listed like any other rather than hidden.
func TestEveryChannelIsOffered(t *testing.T) {
	m := Build(settings(), Idle, false)
	host := find(m, "Keyboard and mouse: off")
	if host == nil {
		t.Fatal("no host control")
	}
	got := []string{}
	for _, c := range host.Children {
		got = append(got, c.Title)
	}
	want := []string{"off", "1", "2", "3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("choices = %v, want %v", got, want)
	}
}

func TestClickingAChannelSaysWhichKeyAndWhichChannel(t *testing.T) {
	m := Build(settings(), Idle, false)
	host := find(m, "Keyboard and mouse: 3")
	for _, c := range host.Children {
		if c.Action != ActionSetHost {
			t.Errorf("%q has action %v, want ActionSetHost", c.Title, c.Action)
		}
		if c.Key != "dp" {
			t.Errorf("%q carries key %q, want dp", c.Title, c.Key)
		}
		if c.Host == "" {
			t.Errorf("%q carries no channel", c.Title)
		}
	}
}

// Clicking a name is one call, and this app never decides the order a switch
// happens in. It is odisplay's job and it is the one thing that must not be
// written twice.
func TestSwitchingIsOneActionThatDecidesNothing(t *testing.T) {
	m := Build(settings(), Idle, false)
	item := find(m, "Mac Mini")
	if item.Action != ActionSwitch {
		t.Fatalf("action = %v, want ActionSwitch", item.Action)
	}
	if item.Key != "dp" {
		t.Errorf("key = %q, want dp", item.Key)
	}
	// Nothing in a switch item reaches into odisplay's steps.
	for _, it := range m.Items {
		if it.Action == ActionSwitch && len(it.Children) != 0 {
			t.Errorf("%q carries children; a switch is one call", it.Title)
		}
	}
}

// While a switch is running nothing can be clicked, so two cannot interleave.
// On a keyboard and mouse that means one machine with a mouse and the other
// without one.
func TestBusyRefusesEveryAction(t *testing.T) {
	m := Build(settings(), Idle, true)
	for _, it := range m.Items {
		if it.Action == ActionSwitch || it.Action == ActionSetHost {
			if !it.Disabled {
				t.Errorf("%q is still clickable while busy", it.Title)
			}
		}
		for _, c := range it.Children {
			if !c.Disabled {
				t.Errorf("%q's choice %q is still clickable while busy", it.Title, c.Title)
			}
		}
	}
}

func TestQuitIsAlwaysAvailable(t *testing.T) {
	m := Build(settings(), Status{Text: "failed", Error: true}, true)
	q := find(m, "Quit")
	if q == nil {
		t.Fatal("no Quit")
	}
	if q.Disabled {
		t.Error("Quit is disabled, so a failed switch could not be escaped")
	}
}

// A failure has to look different from a success, or the menu says the same
// thing either way.
func TestAFailedSwitchIsMarkedAsAnError(t *testing.T) {
	m := Build(settings(), ForSwitch(cli.Result{Code: 3}, "Mac Mini"), false)
	if !m.Status.Error {
		t.Error("a failed switch is not marked as an error")
	}
	if !strings.Contains(m.Status.Text, "did not") {
		t.Errorf("status = %q, want it to say the devices did not move", m.Status.Text)
	}

	ok := Build(settings(), ForSwitch(cli.Result{Code: 0, Message: "Sent Mac Mini."}, "Mac Mini"), false)
	if ok.Status.Error {
		t.Error("a finished switch is marked as an error")
	}
}

// The status line is one line, because a menu item is one line.
func TestTheStatusIsOneLine(t *testing.T) {
	res := cli.Result{Code: 2, Message: "Could not switch.\nDisplay 1 not found."}
	s := ForSwitch(res, "Mac Mini")
	if strings.Contains(s.Text, "\n") {
		t.Errorf("status = %q, want it on one line", s.Text)
	}
}

func TestNoInputsSaysSoRatherThanShowingAnEmptyMenu(t *testing.T) {
	m := Build(cli.Settings{Display: 1}, Idle, false)
	if find(m, "No inputs are configured.") == nil {
		t.Errorf("an empty menu said nothing:\n%s", m.Describe())
	}
}

// Nothing working is the one case that makes a menu bar app look broken, so it
// names where it looked.
func TestNotInstalledSaysWhereItLooked(t *testing.T) {
	m := NotInstalled([]string{"/usr/local/bin/odisplay", "/home/you/.local/bin/odisplay"})
	if !m.Status.Error {
		t.Error("not installed is not marked as an error")
	}
	for _, where := range []string{"/usr/local/bin/odisplay", "/home/you/.local/bin/odisplay"} {
		if !strings.Contains(m.Describe(), where) {
			t.Errorf("does not say it looked in %s:\n%s", where, m.Describe())
		}
	}
}

// Separators go between inputs, not between every item, which would be a wall.
func TestSeparatorsFallBetweenInputs(t *testing.T) {
	m := Build(settings(), Idle, false)
	seps := 0
	for _, it := range m.Items {
		if it.SeparatorBefore {
			seps++
		}
	}
	// One per input, plus one before Reload. Not one per item.
	if seps != 4 {
		t.Errorf("separators = %d, want 4 (one per input, one before Reload)", seps)
	}
}

func TestTheDescriptionRendersTheWholeMenu(t *testing.T) {
	out := Build(settings(), Idle, false).Describe()
	for _, want := range []string{"X", "Mac Mini", "PC", "[x] 3", "Reload settings", "Quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("description is missing %q:\n%s", want, out)
		}
	}
}
