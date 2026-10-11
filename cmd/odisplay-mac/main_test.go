package main

import (
	"testing"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
)

// The menu is rebuilt only when its shape changes, so this is what decides
// whether a click costs a rebuild. Getting it wrong the safe way means an extra
// rebuild; getting it wrong the other way means the menu keeps a stale item.
func TestShapeChangesOnlyWhenTheInputsDo(t *testing.T) {
	cases := []struct {
		name          string
		before, after []string
		installed     [2]bool
		extra         [2]int
		wantSame      bool
	}{
		{
			name:      "nothing changed",
			before:    []string{"usbc", "dp", "hdmi"},
			after:     []string{"usbc", "dp", "hdmi"},
			installed: [2]bool{true, true},
			wantSame:  true,
		},
		{
			name:      "an input was added",
			before:    []string{"usbc", "dp"},
			after:     []string{"usbc", "dp", "hdmi"},
			installed: [2]bool{true, true},
			wantSame:  false,
		},
		{
			name:      "an input was removed",
			before:    []string{"usbc", "dp", "hdmi"},
			after:     []string{"usbc", "hdmi"},
			installed: [2]bool{true, true},
			wantSame:  false,
		},
		{
			name:      "the inputs were reordered",
			before:    []string{"dp", "hdmi"},
			after:     []string{"hdmi", "dp"},
			installed: [2]bool{true, true},
			wantSame:  false,
		},
		{
			name:      "odisplay went away",
			before:    []string{"usbc", "dp"},
			after:     []string{"<not installed>"},
			installed: [2]bool{true, false},
			wantSame:  false,
		},
		{
			name:      "odisplay came back",
			before:    []string{"<not installed>"},
			after:     []string{"usbc", "dp"},
			installed: [2]bool{false, true},
			wantSame:  false,
		},
		{
			name:      "still not installed",
			before:    []string{"<not installed>"},
			after:     []string{"<not installed>"},
			installed: [2]bool{false, false},
			wantSame:  true,
		},
	}
	for _, c := range cases {
		got := sameShape(
			shape(c.before, c.installed[0], c.extra[0]),
			shape(c.after, c.installed[1], c.extra[1]))
		if got != c.wantSame {
			t.Errorf("%s: sameShape = %v, want %v", c.name, got, c.wantSame)
		}
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

// Not installed and no inputs both build a menu, and neither may panic. This is
// the state the app starts in before it has read anything.
func TestShapeOfTheNotInstalledMenu(t *testing.T) {
	got := shape(nil, false, 0)
	if len(got) != 1 || got[0] != "<not installed>" {
		t.Errorf("shape = %v", got)
	}
	// No inputs but installed is a different shape from not installed, or the
	// menu would not be rebuilt when odisplay appears.
	empty := shape(nil, true, 0)
	if len(empty) == len(got) && len(got) == 1 && empty[0] == got[0] {
		t.Error("no inputs and not installed produce the same shape")
	}
}

// The reason line is part of the menu's shape, so a failure appearing or
// clearing costs one rebuild. The text is not, so a different failure message
// does not: that would rebuild the menu every time a switch was reported.
func TestTheReasonChangesTheShape(t *testing.T) {
	keys := []string{"usbc", "dp"}
	if sameShape(shape(keys, true, 0), shape(keys, true, 1)) {
		t.Error("a reason appearing does not change the shape, so it would not be drawn")
	}
	// A reason and its advice are two lines, so the count has to be part of it
	// and not just whether there is one.
	if sameShape(shape(keys, true, 1), shape(keys, true, 2)) {
		t.Error("a second line does not change the shape, so it would not be drawn")
	}
	if !sameShape(shape(keys, true, 2), shape(keys, true, 2)) {
		t.Error("the same shape with a reason differs from itself")
	}
	if sameShape(shape(keys, true, 0), shape(keys, false, 0)) {
		t.Error("installed and not installed are not the same shape")
	}
	// And not installed keeps its reason lines too, so the advice about where
	// to put the binary survives the same rule.
	if !sameShape(shape(nil, false, 1), shape(nil, false, 1)) {
		t.Error("not installed with a line differs from itself")
	}
}
