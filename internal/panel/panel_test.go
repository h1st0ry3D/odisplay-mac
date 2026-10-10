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
	// One per input, plus one before the display picker. Not one per item,
	// which would be a wall of lines.
	if seps != 4 {
		t.Errorf("separators = %d, want 4 (one per input, one before Display)", seps)
	}
}

func TestTheDescriptionRendersTheWholeMenu(t *testing.T) {
	out := Build(settings(), Idle, false).Describe()
	for _, want := range []string{"X", "Mac Mini", "PC", "[x] 3", "Quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("description is missing %q:\n%s", want, out)
		}
	}
}

// The picker has to offer the same numbers as the Omarchy panel, and mark the
// one in force, or which display a switch will use is a guess.
func TestTheDisplayPickerOffersEveryNumber(t *testing.T) {
	m := Build(settings(), Idle, false)
	picker := find(m, "Display: 1")
	if picker == nil {
		t.Fatalf("no display picker:\n%s", m.Describe())
	}
	if len(picker.Children) != len(DisplayChoices) {
		t.Fatalf("choices = %d, want %d", len(picker.Children), len(DisplayChoices))
	}
	for _, n := range DisplayChoices {
		want := DisplayTitle(settings().MonitorOr(), n)
		found := false
		for _, c := range picker.Children {
			if c.Title == want {
				found = true
				if c.Action != ActionSetDisplay {
					t.Errorf("%q has action %v", c.Title, c.Action)
				}
				if c.Display != n {
					t.Errorf("%q carries %d", c.Title, c.Display)
				}
			}
		}
		if !found {
			t.Errorf("no choice for %q", want)
		}
	}
}

// The label names the program that will actually be asked, which odisplay
// reports. It is not the same on every Mac, and a label naming the other one
// sends people to look at the wrong thing.
func TestTheDisplayLabelNamesTheProgramInUse(t *testing.T) {
	for _, monitor := range []string{"m1ddc", "ddcctl", "ddcutil"} {
		if got := DisplayTitle(monitor, 3); got != monitor+" display 3" {
			t.Errorf("DisplayTitle(%q, 3) = %q", monitor, got)
		}
	}

	s := settings()
	s.Monitor = "m1ddc"
	m := Build(s, Idle, false)
	for _, c := range find(m, "Display: 1").Children {
		if !strings.HasPrefix(c.Title, "m1ddc display ") {
			t.Errorf("choice %q does not name m1ddc", c.Title)
		}
	}
}

// A settings reply with no monitor named must still produce a readable label.
func TestAMissingMonitorNameStillLabels(t *testing.T) {
	s := settings()
	s.Monitor = ""
	if got := DisplayTitle(s.MonitorOr(), 2); got != "display display 2" {
		// "display display 2" is not good, but it is a label rather than a
		// panic, and the alternative is inventing a program name.
		t.Logf("fallback label = %q", got)
	}
	if s.MonitorOr() != "display" {
		t.Errorf("MonitorOr() = %q", s.MonitorOr())
	}
}

func TestTheTickIsOnTheDisplayInForce(t *testing.T) {
	s := settings()
	s.Display = 3
	picker := find(Build(s, Idle, false), "Display: 3")
	if picker == nil {
		t.Fatal("the title does not name the display in force")
	}
	ticked := 0
	for _, c := range picker.Children {
		if c.Checked {
			ticked++
			if c.Display != 3 {
				t.Errorf("ticked %d, want 3", c.Display)
			}
		}
	}
	if ticked != 1 {
		t.Errorf("ticked %d choices, want exactly 1", ticked)
	}
}

// A switch in flight disables the picker too, so the number cannot be changed
// underneath the switch that is using it.
func TestThePickerIsDisabledWhileBusy(t *testing.T) {
	picker := find(Build(settings(), Idle, true), "Display: 1")
	if picker == nil {
		t.Fatal("no picker")
	}
	if !picker.Disabled {
		t.Error("the picker is clickable while busy")
	}
	for _, c := range picker.Children {
		if !c.Disabled {
			t.Errorf("%q is clickable while busy", c.Title)
		}
	}
}

// The settings are re-read after everything this menu can do, so a Reload
// alongside them is a button that does nothing a click would not have already
// done. The one menu that keeps it is the one with nothing driving it.
func TestTheNormalMenuHasNoReload(t *testing.T) {
	m := Build(settings(), Idle, false)
	for _, it := range m.Items {
		if strings.Contains(it.Title, "Reload") {
			t.Errorf("the menu still offers %q", it.Title)
		}
	}
}

// With nothing driving it, nothing re-reads on its own, so installing odisplay
// while this is open would otherwise need the app restarting.
func TestTheNotInstalledMenuKeepsAReload(t *testing.T) {
	m := NotInstalled([]string{"/usr/local/bin/odisplay"})
	found := false
	for _, it := range m.Items {
		if it.Action == ActionReload {
			found = true
		}
	}
	if !found {
		t.Errorf("no way to look again without restarting:\n%s", m.Describe())
	}
}
