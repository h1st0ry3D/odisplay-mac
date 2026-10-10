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
	"log"
	"sync"

	"fyne.io/systray"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
	"github.com/h1st0ry3D/odisplay-mac/internal/panel"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("odisplay-mac: ")
	systray.Run(onReady, func() {})
}

// row is one input's two items, kept so they can be edited in place.
type row struct {
	key    string
	label  *systray.MenuItem
	host   *systray.MenuItem
	choice map[string]*systray.MenuItem
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

	// built is what is on screen, so a rebuild can tell a shape change from a
	// value change.
	built    bool
	builtFor []string
	statusIt *systray.MenuItem
	rows     []*row
	reloadIt *systray.MenuItem
	quitIt   *systray.MenuItem
	// all is every item on screen, in the order it appears, so a rebuild can
	// take the whole menu down before putting a new one up.
	all []*systray.MenuItem
}

var state = &app{status: panel.Idle}

func onReady() {
	systray.SetTitle("Odisplay")
	systray.SetTooltip("Odisplay — switch the monitor and the keyboard and mouse")

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

// shape is what a menu is built for: which inputs, or none if odisplay is not
// there. Two settings with the same shape can be drawn by editing titles.
func shape(keys []string, installed bool) []string {
	if !installed {
		return []string{"<not installed>"}
	}
	return keys
}

// render brings the menu up to date.
//
// In place where it can, rebuilt where it must: the shape is stable for
// everything that happens in normal use, so only a settings edit that adds or
// removes an input costs a rebuild.
func (a *app) render() {
	a.mu.Lock()
	client, settings, status, busy := a.client, a.settings, a.status, a.busy
	installed := client != nil
	var keys []string
	for _, in := range settings.Inputs {
		keys = append(keys, in.Key)
	}
	a.mu.Unlock()

	want := shape(keys, installed)
	if !a.built || !sameShape(a.builtFor, want) {
		a.rebuild(installed, settings.Inputs)
	}

	a.mu.Lock()
	a.builtFor = want
	a.built = true
	a.mu.Unlock()

	if !installed {
		return
	}

	var model *panel.Model
	a.mu.Lock()
	model = panel.Build(a.settings, a.status, a.busy)
	a.mu.Unlock()

	if a.statusIt != nil {
		a.statusIt.SetTitle(model.Status.Text)
		a.statusIt.SetTooltip(model.Status.Text)
	}
	if a.reloadIt != nil {
		setEnabled(a.reloadIt, !busy)
	}

	for _, r := range a.rows {
		host := "0"
		label := r.key
		for _, in := range settings.Inputs {
			if in.Key == r.key {
				host = in.HostChannel()
				label = in.Label()
			}
		}
		if r.label != nil {
			r.label.SetTitle(label)
			setEnabled(r.label, !busy)
		}
		if r.host != nil {
			r.host.SetTitle("Keyboard and mouse: " + panel.HostLabel(host))
			r.host.SetTooltip(panel.HostMeaning(host))
			setEnabled(r.host, !busy)
		}
		for h, item := range r.choice {
			if h == host {
				item.Check()
			} else {
				item.Uncheck()
			}
			setEnabled(item, !busy)
		}
	}
	_ = status
}

// setEnabled greys an item out or brings it back.
//
// Everything an input owns is disabled while a switch is in flight, so a second
// cannot be started on top of the first.
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

// rebuild takes the menu down and builds it again.
//
// Removal rather than hiding, because a hidden item is still an item the native
// menu holds. Removing an item also closes its click channel, so the goroutine
// waiting on it ends rather than waiting on something nobody can click.
// A rebuild only happens when the shape changes, which is when the settings
// gain or lose an input, or odisplay appears or disappears.
//
// Everything is rebuilt, Quit included, because systray only appends. Keeping
// one item across a rebuild would leave it above everything added after it.
//
// Each item is created and recorded on the next line, in the order it should
// appear, because the order of these calls is the order of the menu. Keeping
// that readable matters more here than anywhere else in this program: it is the
// part no test on another platform can reach.
func (a *app) rebuild(installed bool, inputs []cli.Input) {
	for _, it := range a.all {
		it.Remove()
	}
	a.all = nil
	a.rows = nil

	// keep records an item and returns it, so a line is one call and one thing
	// to remember rather than two.
	keep := func(it *systray.MenuItem) *systray.MenuItem {
		a.all = append(a.all, it)
		return it
	}

	a.statusIt = keep(a.addLabel(panel.Idle.Text))

	if !installed {
		for _, path := range cli.Candidates() {
			keep(a.addLabel(path))
		}
	} else {
		for _, in := range inputs {
			keep(a.addLabel(""))

			r := &row{key: in.Key, choice: map[string]*systray.MenuItem{}}
			r.label = keep(systray.AddMenuItem(in.Label(),
				"Switch the monitor to "+in.Label()))
			a.wire(r.label, panel.ActionSwitch, in.Key, "")

			r.host = keep(systray.AddMenuItem(
				"Keyboard and mouse: "+panel.HostLabel(in.HostChannel()),
				panel.HostMeaning(in.HostChannel())))
			for _, h := range panel.Hosts {
				c := r.host.AddSubMenuItemCheckbox(
					panel.HostLabel(h), panel.HostMeaning(h), false)
				r.choice[h] = c
				a.wire(c, panel.ActionSetHost, in.Key, h)
			}
			a.rows = append(a.rows, r)
		}
	}

	keep(a.addLabel(""))

	a.reloadIt = keep(systray.AddMenuItem("Reload settings",
		"Re-read the settings, for after a hand edit"))
	a.wire(a.reloadIt, panel.ActionReload, "", "")

	a.quitIt = keep(systray.AddMenuItem("Quit",
		"Leave Odisplay. Nothing is switched on the way out."))
	go func(item *systray.MenuItem) {
		for range item.ClickedCh {
			log.Println("quitting")
			systray.Quit()
			return
		}
	}(a.quitIt)

	log.Printf("menu rebuilt: %d inputs, installed=%v", len(inputs), installed)
}

// wire sends every click on an item to run.
func (a *app) wire(item *systray.MenuItem, action panel.Action, key, host string) {
	go func() {
		for range item.ClickedCh {
			a.run(action, key, host)
		}
	}()
}

// addLabel adds an item nobody can click: a heading, or the blank line that
// stands in for a separator, which systray has no call for.
func (a *app) addLabel(title string) *systray.MenuItem {
	it := systray.AddMenuItem(title, "")
	it.Disable()
	return it
}

// run does one thing, then brings the menu up to date.
//
// busy is what keeps two switches from overlapping. On a keyboard and mouse an
// interleaved pair leaves one machine with the pointer and the other with
// nothing.
func (a *app) run(action panel.Action, key, host string) {
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
		a.setStatus(panel.ForSwitch(res, label))

	case panel.ActionSetHost:
		label := labelFor(settings, key)
		log.Printf("setting %s host to %s", key, host)
		res := client.SetHost(ctx, key, host)
		a.setStatus(panel.ForSetting(res,
			"Keyboard and mouse for "+label+" set to "+panel.HostLabel(host)+"."))

	case panel.ActionReload:
		log.Println("reloading settings")
		a.setStatus(panel.Idle)
	}

	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()

	// Whatever happened, re-read: a refused change must not stay on screen as
	// though it took.
	a.reload()
}

func labelFor(s cli.Settings, key string) string {
	for _, in := range s.Inputs {
		if in.Key == key {
			return in.Label()
		}
	}
	return key
}
