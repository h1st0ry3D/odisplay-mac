package panel

import (
	"os"
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
		"EasySwitch: off",
		"EasySwitch: 3",
		"EasySwitch: off",
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
	host := find(m, "EasySwitch: 3")
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
	host := find(m, "EasySwitch: off")
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
	host := find(m, "EasySwitch: 3")
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

// Renaming has to be reachable per input, the same as the panel's right-click.
//
// It cannot be a child of the input's own item: a menu item with children
// cannot also be clicked, and that item is the switch. So it is a submenu of
// its own, one entry per input, each carrying the key it would rename.
func TestRenameOffersOneEntryPerInput(t *testing.T) {
	m := Build(settings(), Idle, false)
	rename := find(m, "Rename")
	if rename == nil {
		t.Fatalf("no rename entry:\n%s", m.Describe())
	}
	if len(rename.Children) != len(settings().Inputs) {
		t.Fatalf("entries = %d, want %d", len(rename.Children), len(settings().Inputs))
	}
	keys := map[string]bool{}
	for i, c := range rename.Children {
		if c.Action != ActionRename {
			t.Errorf("%q has action %v", c.Title, c.Action)
		}
		if c.Key == "" {
			t.Errorf("%q carries no key, so it would rename nothing", c.Title)
		}
		keys[c.Key] = true
		if !strings.HasSuffix(c.Title, "…") {
			t.Errorf("%q does not hint at a prompt", c.Title)
		}
		// Named after what the input is called now, so the right one is picked
		// without knowing the keys.
		if !strings.HasPrefix(c.Title, settings().Inputs[i].Label()) {
			t.Errorf("%q does not name the input it renames", c.Title)
		}
	}
	for _, want := range []string{"usbc", "dp", "hdmi"} {
		if !keys[want] {
			t.Errorf("no rename entry for %q", want)
		}
	}
}

// An unnamed input is labelled by its key everywhere else, and the rename entry
// has to match or it would look like a different input.
func TestRenameEntryUsesTheSameLabelAsTheRow(t *testing.T) {
	s := cli.Settings{Inputs: []cli.Input{{Key: "dp", VCP: 15}}}
	m := Build(s, Idle, false)
	rename := find(m, "Rename")
	if rename == nil {
		t.Fatal("no rename entry")
	}
	if got := rename.Children[0].Title; got != "dp…" {
		t.Errorf("rename entry = %q, want it to match the row's label", got)
	}
}

func TestRenameIsDisabledWhileBusy(t *testing.T) {
	rename := find(Build(settings(), Idle, true), "Rename")
	if rename == nil {
		t.Fatal("no rename entry")
	}
	if !rename.Disabled {
		t.Error("the rename menu is clickable while busy")
	}
	for _, c := range rename.Children {
		if !c.Disabled {
			t.Errorf("%q is clickable while busy", c.Title)
		}
	}
}

// The label is written once and read by both the model and the code that puts
// items on screen. Two copies drifted before, which is why it moved here.
func TestTheHostLabelSaysEasySwitch(t *testing.T) {
	for host, want := range map[string]string{
		"0": "EasySwitch: off",
		"1": "EasySwitch: 1",
		"2": "EasySwitch: 2",
		"3": "EasySwitch: 3",
		"":  "EasySwitch: off",
	} {
		if got := HostTitle(host); got != want {
			t.Errorf("HostTitle(%q) = %q, want %q", host, got, want)
		}
	}
}

// And the menu uses it, so a change here cannot miss a row.
func TestEveryHostControlUsesTheSameLabel(t *testing.T) {
	m := Build(settings(), Idle, false)
	found := 0
	for _, it := range m.Items {
		if strings.HasPrefix(it.Title, "EasySwitch: ") {
			found++
			if len(it.Children) != len(Hosts) {
				t.Errorf("%q has %d choices, want %d", it.Title, len(it.Children), len(Hosts))
			}
		}
		if strings.Contains(it.Title, "Keyboard and mouse") {
			t.Errorf("the old label is still on %q", it.Title)
		}
	}
	if found != len(settings().Inputs) {
		t.Errorf("%d EasySwitch controls, want one per input (%d)", found, len(settings().Inputs))
	}
}

// The reason a switch failed has to reach the menu.
//
// mxswitch says "Click the mouse to wake it, and check Input Monitoring" when
// it cannot find the device, and odisplay carries that up as the detail. It was
// being dropped, so a failed switch said only that the devices did not move,
// which is the one thing the person already knew.
func TestTheReasonIsShownUnderTheStatus(t *testing.T) {
	res := cli.Result{
		Code:    3,
		Message: "Sent PC, but the devices did not move.",
		Detail:  "No Logitech device supporting ChangeHost found. Click the mouse to wake it, and check Input Monitoring.",
	}
	m := Build(settings(), ForSwitch(res, "PC"), false)

	var detail *Item
	for _, it := range m.Items {
		if it.StatusDetail {
			detail = it
		}
	}
	if detail == nil {
		t.Fatalf("no reason shown:\n%s", m.Describe())
	}
	if !strings.Contains(detail.Title, "Input Monitoring") {
		t.Errorf("reason = %q, want the part that says what to do", detail.Title)
	}
	if !detail.Disabled {
		t.Error("the reason line is clickable")
	}
}

// No reason means no line, or every ordinary switch would grow an empty row.
func TestNoReasonMeansNoLine(t *testing.T) {
	m := Build(settings(), ForSwitch(cli.Result{Code: 0, Message: "Sent PC."}, "PC"), false)
	for _, it := range m.Items {
		if it.StatusDetail {
			t.Errorf("a successful switch shows a reason line: %q", it.Title)
		}
	}
}

// The fallback text is used when odisplay said nothing, and it belongs in the
// same place.
func TestTheFallbackReasonIsShownToo(t *testing.T) {
	m := Build(settings(), ForSwitch(cli.Result{Code: 3}, "PC"), false)
	found := false
	for _, it := range m.Items {
		if it.StatusDetail && strings.Contains(it.Title, "button") {
			found = true
		}
	}
	if !found {
		t.Errorf("no fallback reason shown:\n%s", m.Describe())
	}
}

// Hiding is a view setting. The row goes and the input stays, and the Hide list
// is the only place it appears -- which is what makes hiding undoable.
func TestAHiddenInputGetsNoRow(t *testing.T) {
	s := settings()
	s.Inputs[1].Hidden = true // Mac Mini
	m := Build(s, Idle, false)

	if find(m, "Mac Mini") != nil {
		t.Errorf("a hidden input still has a row:\n%s", m.Describe())
	}
	hide := find(m, "Hide")
	if hide == nil {
		t.Fatalf("no Hide control:\n%s", m.Describe())
	}
	found := false
	for _, c := range hide.Children {
		if c.Title == "Mac Mini" {
			found = true
			if !c.Checked {
				t.Error("a hidden input is not ticked in the Hide list")
			}
		}
	}
	if !found {
		t.Error("a hidden input cannot be reached to unhide it")
	}
}

// Every input is offered, including the visible ones, or the list would shrink
// as things were hidden and there would be no way back to the ones already
// shown.
func TestHideOffersEveryInput(t *testing.T) {
	s := settings()
	s.Inputs[0].Hidden = true
	hide := find(Build(s, Idle, false), "Hide")
	if hide == nil {
		t.Fatal("no Hide control")
	}
	if len(hide.Children) != len(s.Inputs) {
		t.Fatalf("choices = %d, want %d", len(hide.Children), len(s.Inputs))
	}
	shown := 0
	for _, c := range hide.Children {
		if !c.Checked {
			shown++
		}
	}
	if shown != len(s.Inputs)-1 {
		t.Errorf("%d unticked, want one tick per hidden input", shown)
	}
}

// Nothing hidden means every row and an unticked list, so a menu that has never
// used this looks exactly as it did before.
func TestNothingHiddenLooksUnchanged(t *testing.T) {
	m := Build(settings(), Idle, false)
	for _, in := range settings().Inputs {
		if find(m, in.Label()) == nil {
			t.Errorf("%q has no row", in.Label())
		}
	}
	for _, c := range find(m, "Hide").Children {
		if c.Checked {
			t.Errorf("%q is ticked with nothing hidden", c.Title)
		}
	}
}

// Hiding the input that is in use is the point, so the row list has to survive
// losing every one of them.
func TestHidingEveryInputLeavesAUsableMenu(t *testing.T) {
	s := settings()
	for i := range s.Inputs {
		s.Inputs[i].Hidden = true
	}
	m := Build(s, Idle, false)
	if find(m, "Hide") == nil {
		t.Fatalf("no Hide control to undo from:\n%s", m.Describe())
	}
	if find(m, "No inputs are configured.") != nil {
		t.Error("inputs exist but the menu says there are none")
	}
}

// Every input gets a Power control under its channel, because the two are the
// two things a switch does: send the keyboard and mouse, then decide what
// becomes of this machine.
func TestEveryInputHasAPowerControl(t *testing.T) {
	m := Build(settings(), Idle, false)
	controls := 0
	for _, it := range m.Items {
		if !strings.HasPrefix(it.Title, "Power: ") {
			continue
		}
		controls++
		if len(it.Children) != len(PowerChoices) {
			t.Errorf("%q has %d choices, want %d", it.Title, len(it.Children), len(PowerChoices))
		}
	}
	if controls != len(settings().Inputs) {
		t.Errorf("%d Power controls, want one per input (%d)", controls, len(settings().Inputs))
	}
}

// It sits directly under the channel control of the same input, not grouped
// with the others somewhere else.
func TestPowerSitsUnderItsOwnChannel(t *testing.T) {
	m := Build(settings(), Idle, false)
	var order []string
	for _, it := range m.Items {
		switch {
		case strings.HasPrefix(it.Title, "EasySwitch: "):
			order = append(order, "channel")
		case strings.HasPrefix(it.Title, "Power: "):
			order = append(order, "power")
		}
	}
	want := []string{"channel", "power", "channel", "power", "channel", "power"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// Nothing set reads as Awake, and it is ticked, so a menu nobody has configured
// says what the machine will do rather than leaving a blank.
func TestTheActionInForceIsTicked(t *testing.T) {
	cases := []struct{ power, label string }{
		{"", "Awake"},
		{"sleep", "Sleep"},
		{"shutdown", "Shutdown"},
	}
	for _, c := range cases {
		s := settings()
		s.Inputs[0].Power = c.power
		m := Build(s, Idle, false)
		item := find(m, "Power: "+c.label)
		if item == nil {
			t.Errorf("power %q has no control reading %q:\n%s", c.power, c.label, m.Describe())
			continue
		}
		ticked := 0
		for _, choice := range item.Children {
			if choice.Checked {
				ticked++
				if choice.Power != c.power {
					t.Errorf("power %q ticked %q", c.power, choice.Power)
				}
			}
		}
		if ticked != 1 {
			t.Errorf("power %q has %d ticks, want one", c.power, ticked)
		}
	}
}

// A value odisplay would read as doing nothing reads as Awake here too, rather
// than as a fourth state nobody can select again.
func TestAnUnknownActionReadsAsAwake(t *testing.T) {
	if got := PowerTitle("hibernate"); got != "Power: Awake" {
		t.Errorf("PowerTitle(hibernate) = %q, want Awake", got)
	}
	s := settings()
	s.Inputs[0].Power = "hibernate"
	if find(Build(s, Idle, false), "Power: Awake") == nil {
		t.Error("an unknown action leaves the control unlabelled")
	}
}

func TestPowerIsDisabledWhileBusy(t *testing.T) {
	m := Build(settings(), Idle, true)
	for _, it := range m.Items {
		if !strings.HasPrefix(it.Title, "Power: ") {
			continue
		}
		if !it.Disabled {
			t.Errorf("%q is clickable while busy", it.Title)
		}
		for _, c := range it.Children {
			if !c.Disabled {
				t.Errorf("%q is clickable while busy", c.Title)
			}
		}
	}
}

// The wording is the machine's, because that is what the setting is about.
func TestThePowerChoicesAreTheThreeStates(t *testing.T) {
	var labels []string
	for _, c := range PowerChoices {
		labels = append(labels, c.Label)
	}
	if strings.Join(labels, ",") != "Awake,Sleep,Shutdown" {
		t.Errorf("labels = %v, want Awake, Sleep and Shutdown", labels)
	}
}

// The README draws the menu, and it drifted: it still showed one Power control
// grouped with Hide after the feature had become one per input. A drawing that
// disagrees with the menu is worse than none, because it is read instead of the
// code. This is the narrow part that drifted, so it is the narrow part checked.
func TestTheReadmeDrawsTheMenuAsItIs(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	// The first fenced block is the menu.
	body := string(raw)
	start := strings.Index(body, "```\n")
	if start < 0 {
		t.Fatal("the README has no menu block")
	}
	rest := body[start+4:]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("the README's menu block is not closed")
	}
	lines := strings.Split(rest[:end], "\n")

	// Every channel line is followed by its input's Power line, and no Power
	// line stands anywhere else.
	channels, powers := 0, 0
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "  EasySwitch:"):
			channels++
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "  Power: ") {
				t.Errorf("the README's diagram has no Power line under %q", line)
			}
		case strings.HasPrefix(line, "  Power: "):
			powers++
			if i == 0 || !strings.HasPrefix(lines[i-1], "  EasySwitch:") {
				t.Errorf("the README draws %q as a standalone menu, not under an input", line)
			}
		}
	}
	if channels == 0 || channels != powers {
		t.Errorf("the README draws %d channels and %d Power controls, want the same number, one each",
			channels, powers)
	}
}

// systray cannot turn an ordinary entry into a checkbox once it is made, so the
// model has to say which entries are tickable before the menu is built. Every
// group that carries a tick is checked here, so one that gains a tick and
// forgets the flag is a failing test rather than a choice with nowhere to put
// it.
func TestTheEntriesThatCarryATickAreCheckable(t *testing.T) {
	ticks := 0
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			// A submenu heading cannot be clicked, so it carries no action and
			// no tick, and is not one of these.
			leaf := len(it.Children) == 0
			if it.Checked {
				ticks++
				if !it.Checkable {
					t.Errorf("%q is ticked and is not a checkbox", it.Title)
				}
			}
			switch {
			case !leaf:
			case it.Action == ActionSetHost, it.Action == ActionSetPower,
				it.Action == ActionToggleHidden, it.Action == ActionSetDisplay:
				if !it.Checkable {
					t.Errorf("%q writes a setting that is read back as a tick, so it has to be a checkbox",
						it.Title)
				}
			case it.Action == ActionRename:
				// Renaming opens a dialog. A tick beside it would mean nothing.
				if it.Checkable {
					t.Errorf("%q opens a dialog and must not be a checkbox", it.Title)
				}
			}
			walk(it.Children)
		}
	}
	walk(Build(settings(), Idle, false).Items)
	if ticks == 0 {
		t.Error("nothing in the menu is ticked, so this checked nothing")
	}
}
