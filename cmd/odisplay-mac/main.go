// Command odisplay-mac puts a monitor input switcher in the macOS menu bar.
//
// It is a front end. Everything that matters happens in odisplay: which
// commands run, and in what order. This program decides how to draw a list and
// nothing else, so it cannot put the monitor move after the keyboard and mouse
// move, because it never sees the individual steps.
//
// One switch is one call. Clicking a name asks odisplay to switch; choosing a
// channel asks it to remember a different channel. The second only writes a
// setting, and nothing moves until that input is switched to.
//
// The menu is built once and then edited in place. systray adds items rather
// than replacing them, so rebuilding on every click would leave the previous
// menus underneath the new one for as long as the app runs. A rebuild happens
// only when the shape changes, which is when the settings gain or lose an
// input, or odisplay appears or disappears.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"fyne.io/systray"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
	"github.com/h1st0ry3D/odisplay-mac/internal/dialog"
	"github.com/h1st0ry3D/odisplay-mac/internal/icon"
	"github.com/h1st0ry3D/odisplay-mac/internal/panel"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("odisplay-mac: ")
	systray.Run(onReady, func() {})
}

var state = &app{status: panel.Idle}

func onReady() {
	// The glyph, not the word. systray draws only the image when the title is
	// empty, so this is the Omarchy bar's own fa-tv in the menu bar rather than
	// a name next to it.
	//
	// It is handed over as a template image: black with an alpha channel, which
	// macOS inverts for a dark menu bar and leaves alone for a light one. The
	// same bytes go in both arguments because on macOS only the first is used,
	// and the second is what other platforms would draw.
	systray.SetTemplateIcon(icon.TV(), icon.TV())
	systray.SetTitle("")
	systray.SetTooltip("Odisplay: switch the monitor and the keyboard and mouse")

	state.reload()
}

// reload re-reads the settings and updates the menu.
//
// It runs at start, after every change, and from the Reload item, so a hand
// edit of the settings file takes effect without restarting anything.
func (a *app) reload() {
	client, err := cli.New()
	if err != nil {
		log.Printf("%v", err)
		a.mu.Lock()
		a.client = nil
		a.mu.Unlock()
		a.render()
		return
	}

	settings, err := client.List(context.Background())
	if err != nil {
		log.Printf("could not read the settings: %v", err)
		a.mu.Lock()
		a.client = client
		a.mu.Unlock()
		a.setStatus(panel.Status{Text: "Could not read the odisplay settings.", Error: true})
		return
	}

	a.mu.Lock()
	a.client = client
	a.settings = settings
	a.mu.Unlock()
	a.render()
}

// setStatus records the last result and puts the menu back with it.
func (a *app) setStatus(s panel.Status) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.render()
}
func setEnabled(it *systray.MenuItem, enabled bool) {
	if enabled {
		it.Enable()
	} else {
		it.Disable()
	}
}

// sameShape reports whether two menus are built for the same inputs.
func sameShape(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// app is the live state.
//
// systray runs its callbacks on its own goroutine, and a switch can take the
// length of a Solaar timeout, so the state and the drawing are kept apart by
// one mutex rather than by hoping.
type app struct {
	mu       sync.Mutex
	client   *cli.Client
	settings cli.Settings
	status   panel.Status
	busy     bool

	// drawn is the shape of the model that is on screen. The menu is rebuilt
	// only when that changes, because a rebuild closes whatever submenu is
	// open, and a switch changes the status text on every click.
	drawn []string
	// roots are the items a rebuild takes down, and all is every item in the
	// order the model walks, which is how a render finds one again to edit it.
	roots []*systray.MenuItem
	all   []*systray.MenuItem
}

// render puts the current state on screen.
//
// There is one description of this menu, in the panel package, and this draws
// it. The Omarchy panel draws the same model in its own way, so a change to what
// a row says cannot reach one front end and miss the other.
func (a *app) render() {
	a.mu.Lock()
	settings, status, busy, installed := a.settings, a.status, a.busy, a.client != nil
	a.mu.Unlock()

	var model *panel.Model
	if installed {
		model = panel.Build(settings, status, busy)
	} else {
		model = panel.NotInstalled(cli.Candidates())
	}

	want := shapeOf(model.Items)
	if !sameShape(a.drawn, want) {
		for _, it := range a.roots {
			it.Remove()
		}
		a.draw(model)
		a.drawn = want
	}
	apply(model, a.all)
}

// draw puts the model on screen in one depth-first walk, recording every item it
// makes so a later render can find it again.
//
// roots is what a rebuild takes down, which is only the top level: a menu bar
// removes a submenu with the entry it hangs from. all is every item, in the
// order the model walks, which is the order apply reads them back in.
func (a *app) draw(model *panel.Model) {
	a.roots, a.all = nil, nil

	keep := func(parent *systray.MenuItem, item *systray.MenuItem) {
		a.all = append(a.all, item)
		if parent == nil {
			a.roots = append(a.roots, item)
		}
	}

	var walk func(parent *systray.MenuItem, items []*panel.Item)
	walk = func(parent *systray.MenuItem, items []*panel.Item) {
		for _, it := range items {
			if it.SeparatorBefore {
				keep(parent, a.blank(parent))
			}
			item := a.create(parent, it)
			keep(parent, item)
			a.wire(item, callOf(it))
			walk(item, it.Children)
		}
	}
	walk(nil, model.Items)

	log.Printf("menu rebuilt: %s", model.Summary())
}

// create makes one item from the model.
func (a *app) create(parent *systray.MenuItem, it *panel.Item) *systray.MenuItem {
	var item *systray.MenuItem
	switch {
	case parent == nil:
		item = systray.AddMenuItem(it.Title, it.Tooltip)
	case it.Checkable:
		item = parent.AddSubMenuItemCheckbox(it.Title, it.Tooltip, it.Checked)
	default:
		item = parent.AddSubMenuItem(it.Title, it.Tooltip)
	}
	setEnabled(item, !it.Disabled)
	return item
}

// blank is the line between blocks. A menu bar has no separator of its own for
// this, so it is an empty item that cannot be clicked.
func (a *app) blank(parent *systray.MenuItem) *systray.MenuItem {
	var item *systray.MenuItem
	if parent == nil {
		item = systray.AddMenuItem("", "")
	} else {
		item = parent.AddSubMenuItem("", "")
	}
	setEnabled(item, false)
	return item
}

// callOf is what clicking an item does. The model carries every argument the
// action needs, which is why this is a copy and not a lookup by key.
func callOf(it *panel.Item) call {
	return call{
		action:  it.Action,
		key:     it.Key,
		host:    it.Host,
		display: it.Display,
		power:   it.Power,
	}
}

// apply gives every item on screen what the model says it should have.
//
// The two are walked the same way, so the nth item of the walk is the nth on
// screen, which the shape check is what keeps true. A title is set on the item
// that is there rather than the item replaced, so a new status message does not
// close the submenu the click came from.
func apply(model *panel.Model, live []*systray.MenuItem) {
	at := 0
	var walk func(items []*panel.Item)
	walk = func(items []*panel.Item) {
		for _, it := range items {
			if it.SeparatorBefore {
				at++ // the blank item above has nothing to set
			}
			if at >= len(live) {
				return
			}
			item := live[at]
			at++
			item.SetTitle(it.Title)
			item.SetTooltip(it.Tooltip)
			setEnabled(item, !it.Disabled)
			if it.Checkable {
				if it.Checked {
					item.Check()
				} else {
					item.Uncheck()
				}
			}
			walk(it.Children)
		}
	}
	walk(model.Items)
}

// shapeOf is the structure of a menu: one line per item, in the order it is
// walked, saying where it sits and what it is.
//
// What it is means what clicking it does and what it does that to, so that
// reordering two inputs is a different shape. A title is deliberately not part
// of it: a title changes on every switch and on every rename, and a menu
// rebuilt for one would close the submenu the click came from. That is what
// apply is for.
func shapeOf(items []*panel.Item) []string {
	var out []string
	var walk func(items []*panel.Item, depth int)
	walk = func(items []*panel.Item, depth int) {
		for _, it := range items {
			out = append(out, fmt.Sprintf("%d %d %s %s %d %s %t %t %d",
				depth, int(it.Action), it.Key, it.Host, it.Display, it.Power,
				it.Checkable, it.SeparatorBefore, len(it.Children)))
			walk(it.Children, depth+1)
		}
	}
	walk(items, 0)
	return out
}

// call is everything a menu item needs to say what it does when clicked.
//
// One value rather than five parameters: four of them are strings that mean
// different things, and a call site that got two of them the wrong way round
// would still compile.
type call struct {
	action  panel.Action
	key     string
	host    string
	display int
	power   string
}

// wire sends every click on an item to run.
func (a *app) wire(item *systray.MenuItem, c call) {
	go func() {
		for range item.ClickedCh {
			a.run(c)
		}
	}()
}

// run does one thing, then brings the menu up to date.
//
// busy is what keeps two switches from overlapping. On a keyboard and mouse an
// interleaved pair leaves one machine with the pointer and the other with
// nothing.
func (a *app) run(c call) {
	// The two that work with nothing driving the menu. The menu shown when
	// odisplay is missing offers a reload, because nothing else re-reads once
	// it appears, and quit has to work in that state too.
	switch c.action {
	case panel.ActionQuit:
		log.Println("quitting")
		systray.Quit()
		return
	case panel.ActionReload:
		a.setStatus(panel.Idle)
		a.reload()
		return
	}

	action, key, host, display := c.action, c.key, c.host, c.display
	a.mu.Lock()
	if a.busy || a.client == nil {
		a.mu.Unlock()
		return
	}
	a.busy = true
	client, settings := a.client, a.settings
	a.mu.Unlock()

	a.render()

	ctx := context.Background()
	switch action {
	case panel.ActionSwitch:
		label := labelFor(settings, key)
		log.Printf("switching to %s (%s)", label, key)
		res := client.Switch(ctx, key)
		a.setStatus(withHint(panel.ForSwitch(res, label), res))

	case panel.ActionSetHost:
		label := labelFor(settings, key)
		log.Printf("setting %s host to %s", key, host)
		res := client.SetHost(ctx, key, host)
		a.setStatus(panel.ForSetting(res,
			"EasySwitch for "+label+" set to "+panel.HostLabel(host)+"."))

	case panel.ActionRename:
		in, ok := inputByKey(settings, key)
		if !ok {
			break
		}
		// The dialog is the whole interaction. Dismissing it is an answer, so
		// it leaves both the name and the line alone rather than reporting
		// that nothing happened.
		name, asked, err := dialog.Ask(ctx,
			"Name for this input. Empty restores the default.", in.Name)
		if err != nil || !asked {
			if err != nil && !errors.Is(err, dialog.ErrCancelled) {
				log.Printf("rename dialog: %v", err)
				// The reason goes in the menu, not only in the log. A log from
				// an app launched by Finder is somewhere nobody looks, and
				// "could not open the dialog" on its own is not something
				// anyone can act on.
				a.setStatus(panel.Status{
					Text:  "Rename: " + oneLine(err.Error()),
					Error: true,
				})
			}
			break
		}
		log.Printf("renaming %s to %q", key, name)
		res := client.SetName(ctx, key, name)
		a.setStatus(panel.ForSetting(res, "Renamed to "+nameOr(in, name)+"."))

	case panel.ActionToggleHidden:
		in, ok := inputByKey(settings, key)
		if !ok {
			break
		}
		log.Printf("hiding=%v for %s", !in.Hidden, key)
		// Setting rather than toggling, so what is written is what this
		// decided from what it had read. A toggle would depend on the two
		// agreeing about the current state.
		res := client.SetHidden(ctx, key, !in.Hidden)
		what := in.Label() + " shown in the menu again."
		if !in.Hidden {
			what = in.Label() + " hidden from the menu."
		}
		a.setStatus(panel.ForSetting(res, what))

	case panel.ActionSetPower:
		log.Printf("power=%q for %s", c.power, key)
		res := client.SetPower(ctx, key, c.power)
		what := "This Mac will stay awake."
		switch c.power {
		case "sleep":
			what = "This Mac will sleep after a switch."
		case "shutdown":
			what = "This Mac will shut down after a switch."
		}
		a.setStatus(panel.ForSetting(res, what))

	case panel.ActionSetDisplay:
		log.Printf("setting the display to %d", display)
		res := client.SetDisplay(ctx, display)
		a.setStatus(panel.ForSetting(res,
			"Switching "+panel.DisplayTitle(settings.MonitorOr(), display)+" from now on."))

	}

	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()

	// Whatever happened, re-read: a refused change must not stay on screen as
	// though it took.
	a.reload()
}

// withHint adds what this app knows and odisplay cannot.
//
// macOS decides whether a program may open a Logitech device by looking at the
// app that started the chain, not at mxswitch. So a switch run from a terminal
// works -- the terminal is what is asked -- and the same switch from here is
// asked of this app. Rebuilding it changes its code signature, and the
// permission macOS granted does not survive that, which is why this works until
// the next build and then stops.
//
// Worth saying out loud because the failure looks identical to a device that is
// asleep, and one of those is fixed by pressing a button.
func withHint(st panel.Status, res cli.Result) panel.Status {
	if res.Outcome() != cli.OutcomeDevicesFailed {
		return st
	}
	st.Hint = "macOS asks this app for that permission, not mxswitch, and forgets it when " +
		"the app is rebuilt. Remove Odisplay from Privacy & Security > Input Monitoring " +
		"and add it again."
	return st
}

// oneLine shortens a message to something a menu item can hold. A menu item
// ellipsises its middle, which would cut out the part that says what happened.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const limit = 120
	if len(s) > limit {
		return s[:limit-1] + "…"
	}
	return s
}

// nameOr is what the status line says after a rename. An empty name means the
// built-in label is back, so saying "Renamed to ." would read as a failure.
func nameOr(in cli.Input, name string) string {
	if strings.TrimSpace(name) == "" {
		return in.Key + " (default)"
	}
	return name
}

func labelFor(s cli.Settings, key string) string {
	if in, ok := inputByKey(s, key); ok {
		return in.Label()
	}
	return key
}

// inputByKey finds one input, so a caller does not have to walk the list and
// then remember whether it found anything.
func inputByKey(s cli.Settings, key string) (cli.Input, bool) {
	for _, in := range s.Inputs {
		if in.Key == key {
			return in, true
		}
	}
	return cli.Input{}, false
}
