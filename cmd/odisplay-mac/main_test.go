package main

import (
	"testing"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
	"github.com/h1st0ry3D/odisplay-mac/internal/panel"
)

// settings builds a settings file with the named inputs, for the shape tests.
func settings(names ...string) cli.Settings {
	s := cli.Settings{Display: 1, Monitor: "m1ddc"}
	for i, n := range names {
		s.Inputs = append(s.Inputs, cli.Input{Key: n, Name: n, VCP: 15 + i, Host: "0"})
	}
	return s
}

// shapeOfMenu is the shape of the menu the app would draw for a state. It is
// read from the model, which is what goes on screen, so these are the items the
// menu is built from rather than a description of them.
func shapeOfMenu(s cli.Settings, st panel.Status) []string {
	return shapeOf(panel.Build(s, st, false).Items)
}

// The menu is rebuilt only when its shape changes, which is what decides whether
// a click costs a rebuild. Getting it wrong the safe way means one extra rebuild;
// getting it wrong the other way means the menu keeps a stale item.
func TestTheShapeChangesOnlyWhenTheMenuDoes(t *testing.T) {
	cases := []struct {
		name          string
		before, after []string
		wantSame      bool
	}{
		{"nothing changed",
			shapeOfMenu(settings("usbc", "dp", "hdmi"), panel.Idle),
			shapeOfMenu(settings("usbc", "dp", "hdmi"), panel.Idle), true},
		{"an input was added",
			shapeOfMenu(settings("usbc", "dp"), panel.Idle),
			shapeOfMenu(settings("usbc", "dp", "hdmi"), panel.Idle), false},
		{"an input was removed",
			shapeOfMenu(settings("usbc", "dp", "hdmi"), panel.Idle),
			shapeOfMenu(settings("usbc", "hdmi"), panel.Idle), false},
		{"the inputs were reordered",
			shapeOfMenu(settings("dp", "hdmi"), panel.Idle),
			shapeOfMenu(settings("hdmi", "dp"), panel.Idle), false},
	}
	for _, c := range cases {
		if got := sameShape(c.before, c.after); got != c.wantSame {
			t.Errorf("%s: sameShape = %v, want %v", c.name, got, c.wantSame)
		}
	}
}

// A status message must not cost a rebuild: it is set on the line that is
// already there, so the submenu the click came from stays open. A reason is a
// line of its own, so one appearing does, and its text does not.
func TestOnlyTheNumberOfStatusLinesChangesTheShape(t *testing.T) {
	s := settings("usbc", "dp")
	failed := panel.Status{Text: "Sent Mac Mini.", Error: true, Detail: "no online device found"}

	if !sameShape(shapeOfMenu(s, panel.Idle), shapeOfMenu(s, panel.Status{Text: "Sent DP."})) {
		t.Error("a different message changes the shape, so every switch would rebuild the menu")
	}
	if sameShape(shapeOfMenu(s, panel.Idle), shapeOfMenu(s, failed)) {
		t.Error("a reason appearing does not change the shape, so it would not be drawn")
	}
	other := failed
	other.Detail = "something else went wrong"
	if !sameShape(shapeOfMenu(s, failed), shapeOfMenu(s, other)) {
		t.Error("a different reason changes the shape, so every failure would rebuild the menu")
	}
	// Advice is a second line, so the count is part of the shape and not just
	// whether there is one.
	advised := failed
	advised.Hint = "Rebuild it and grant the permission again."
	if sameShape(shapeOfMenu(s, failed), shapeOfMenu(s, advised)) {
		t.Error("a second line does not change the shape, so the advice would not be drawn")
	}
}

// A menu with no odisplay in it is still a menu, and a different one from a
// settings file with no inputs in it: the first has to be replaced when odisplay
// appears, and the second is what an empty settings file looks like.
func TestTheNotInstalledMenuIsItsOwnShape(t *testing.T) {
	missing := shapeOf(panel.NotInstalled(cli.Candidates()).Items)
	if len(missing) == 0 {
		t.Fatal("the menu with no odisplay in it has no items")
	}
	empty := shapeOfMenu(settings(), panel.Idle)
	if sameShape(missing, empty) {
		t.Error("no inputs and no odisplay have the same shape, so installing it would not redraw")
	}
}

// A key that is not in the settings has to come back as itself rather than as
// an empty label, or a row with a stale key would render blank.
func TestLabelForFallsBackToTheKey(t *testing.T) {
	s := cli.Settings{Inputs: []cli.Input{
		{Key: "dp", Name: "Mac Mini"},
		{Key: "hdmi"},
	}}
	if got := labelFor(s, "dp"); got != "Mac Mini" {
		t.Errorf("labelFor(dp) = %q", got)
	}
	if got := labelFor(s, "hdmi"); got != "hdmi" {
		t.Errorf("labelFor(hdmi) = %q, want the key", got)
	}
	if got := labelFor(s, "gone"); got != "gone" {
		t.Errorf("labelFor(gone) = %q, want the key", got)
	}
}
