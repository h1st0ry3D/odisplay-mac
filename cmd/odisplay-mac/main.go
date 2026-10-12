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
	"log"
	"strconv"
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

// row is one input's two items, kept so they can be edited in place.
type row struct {
	key    string
	label  *systray.MenuItem
	host   *systray.MenuItem
	choice map[string]*systray.MenuItem
	// power is what this Mac does afterwards, under the channel control.
	power       *systray.MenuItem
	powerChoice map[string]*systray.MenuItem
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
	// statusLines are the reason a switch failed and any advice about it.
	statusLines []*systray.MenuItem
	rows        []*row
	// renameIt is the rename submenu, one entry per input.
	renameIt     *systray.MenuItem
	renameChoice map[string]*systray.MenuItem
	// hideIt is the hide submenu, one entry per input including the hidden
	// ones, which is the only place they appear.
	hideIt     *systray.MenuItem
	hideChoice map[string]*systray.MenuItem
	// displayIt is the picker and its choices, kept so the tick can move and
	// the title can name the number in force.
	displayIt     *systray.MenuItem
	displayChoice map[int]*systray.MenuItem
	reloadIt      *systray.MenuItem
	quitIt        *systray.MenuItem
	// all is every item on screen, in the order it appears, so a rebuild can
	// take the whole menu down before putting a new one up.
	all []*systray.MenuItem
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
func shape(keys []string, installed bool, extraLines int) []string {
	if !installed {
		out := []string{"<not installed>"}
		for i := 0; i < extraLines; i++ {
			out = append(out, "<reason>")
		}
		return out
	}
	out := append([]string(nil), keys...)
	// How many lines a failure adds is part of the shape, so one appearing or
	// clearing costs a rebuild. Their text is not, so a different message does
	// not.
	for i := 0; i < extraLines; i++ {
		out = append(out, "<reason>")
	}
	return out
}

// render brings the menu up to date.
//
// In place where it can, rebuilt where it must: the shape is stable for
// everything that happens in normal use, so only a settings edit that adds or
// removes an input costs a rebuild.
//
// Everything it needs is taken under one lock and then used without it. The
// calls below add and edit menu items, which is not something to do while
// holding a mutex the rest of the program wants.
func (a *app) render() {
	a.mu.Lock()
	settings, status, busy := a.settings, a.status, a.busy
	installed := a.client != nil
	shown := settings.Visible()
	keys := make([]string, 0, len(shown))
	for _, in := range shown {
		keys = append(keys, in.Key)
	}
	a.mu.Unlock()

	lines := statusLines(status)
	want := shape(keys, installed, len(lines))
	if !a.built || !sameShape(a.builtFor, want) {
		a.rebuild(installed, settings.Inputs, settings.MonitorOr(), len(lines))
	}
	a.built, a.builtFor = true, want

	if !installed {
		return
	}

	model := panel.Build(settings, status, busy)

	a.statusIt.SetTitle(model.Status.Text)
	a.statusIt.SetTooltip(model.Status.Text)
	for i, it := range a.statusLines {
		it.SetTitle(lines[i])
		it.SetTooltip(lines[i])
	}

	if a.renameIt != nil {
		setEnabled(a.renameIt, !busy)
		for _, in := range settings.Inputs {
			if c := a.renameChoice[in.Key]; c != nil {
				c.SetTitle(in.Label() + "…")
				setEnabled(c, !busy)
			}
		}
	}

	if a.hideIt != nil {
		setEnabled(a.hideIt, !busy)
		for _, in := range settings.Inputs {
			c := a.hideChoice[in.Key]
			if c == nil {
				continue
			}
			c.SetTitle(in.Label())
			c.SetTooltip(panel.HiddenTooltip(in))
			if in.Hidden {
				c.Check()
			} else {
				c.Uncheck()
			}
			setEnabled(c, !busy)
		}
	}

	if a.displayIt != nil {
		a.displayIt.SetTitle("Display: " + strconv.Itoa(settings.Display))
		setEnabled(a.displayIt, !busy)
		for n, item := range a.displayChoice {
			if n == settings.Display {
				item.Check()
			} else {
				item.Uncheck()
			}
			setEnabled(item, !busy)
		}
	}

	for _, r := range a.rows {
		in, _ := inputByKey(settings, r.key)
		host := in.HostChannel()
		r.label.SetTitle(in.Label())
		setEnabled(r.label, !busy)
		r.host.SetTitle(panel.HostTitle(host))
		r.host.SetTooltip(panel.HostMeaning(host))
		setEnabled(r.host, !busy)
		for h, item := range r.choice {
			if h == host {
				item.Check()
			} else {
				item.Uncheck()
			}
			setEnabled(item, !busy)
		}

		if r.power != nil {
			r.power.SetTitle(panel.PowerTitle(in.Power))
			setEnabled(r.power, !busy)
			for _, choice := range panel.PowerChoices {
				item, ok := r.powerChoice[choice.Value]
				if !ok {
					continue
				}
				if choice.Value == in.Power {
					item.Check()
				} else {
					item.Uncheck()
				}
				setEnabled(item, !busy)
			}
		}
	}
}

// statusLines are the lines under the status itself: the reason a switch gave
// up, and any advice about it. One function, so the count the menu is built
// from and the text put into it cannot disagree.
func statusLines(st panel.Status) []string {
	var out []string
	if d := oneLine(st.Detail); d != "" {
		out = append(out, d)
	}
	if h := oneLine(st.Hint); h != "" {
		out = append(out, h)
	}
	return out
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
func (a *app) rebuild(installed bool, inputs []cli.Input, monitor string, lines int) {
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
	// Right under the status line, where the Omarchy panel puts it too.
	a.statusLines = nil
	for i := 0; i < lines; i++ {
		a.statusLines = append(a.statusLines, keep(a.addLabel("")))
	}

	if !installed {
		for _, path := range cli.Candidates() {
			keep(a.addLabel(path))
		}
	} else {
		for _, in := range visible(inputs) {
			keep(a.addLabel(""))

			r := &row{key: in.Key, choice: map[string]*systray.MenuItem{}}
			r.label = keep(systray.AddMenuItem(in.Label(),
				"Switch the monitor to "+in.Label()))
			a.wire(r.label, call{action: panel.ActionSwitch, key: in.Key})

			r.host = keep(systray.AddMenuItem(
				panel.HostTitle(in.HostChannel()),
				panel.HostMeaning(in.HostChannel())))
			for _, h := range panel.Hosts {
				c := r.host.AddSubMenuItemCheckbox(
					panel.HostLabel(h), panel.HostMeaning(h), false)
				r.choice[h] = c
				a.wire(c, call{action: panel.ActionSetHost, key: in.Key, host: h})
			}

			r.power = keep(systray.AddMenuItem(panel.PowerTitle(in.Power),
				"What this Mac does once the switch is done"))
			r.powerChoice = map[string]*systray.MenuItem{}
			for _, choice := range panel.PowerChoices {
				c := r.power.AddSubMenuItemCheckbox(
					choice.Label, choice.Why, choice.Value == in.Power)
				r.powerChoice[choice.Value] = c
				a.wire(c, call{action: panel.ActionSetPower, key: in.Key, power: choice.Value})
			}
			a.rows = append(a.rows, r)
		}
	}

	keep(a.addLabel(""))

	a.renameIt = keep(systray.AddMenuItem("Rename", "Give an input a name of your own"))
	a.renameChoice = map[string]*systray.MenuItem{}
	if installed {
		for _, in := range inputs {
			c := a.renameIt.AddSubMenuItem(in.Label()+"…", "Rename "+in.Label())
			a.renameChoice[in.Key] = c
			a.wire(c, call{action: panel.ActionRename, key: in.Key})
		}
	}
	setEnabled(a.renameIt, installed)

	a.hideIt = keep(systray.AddMenuItem("Hide", "Keep an input out of this menu without removing it"))
	a.hideChoice = map[string]*systray.MenuItem{}
	if installed {
		for _, in := range inputs {
			c := a.hideIt.AddSubMenuItemCheckbox(in.Label(), panel.HiddenTooltip(in), in.Hidden)
			a.hideChoice[in.Key] = c
			a.wire(c, call{action: panel.ActionToggleHidden, key: in.Key})
		}
	}
	setEnabled(a.hideIt, installed)

	a.displayIt = keep(systray.AddMenuItem("Display", "Which display this machine switches"))
	a.displayChoice = map[int]*systray.MenuItem{}
	for _, n := range panel.DisplayChoices {
		c := a.displayIt.AddSubMenuItemCheckbox(
			panel.DisplayTitle(monitor, n),
			"Switch this monitor, numbered "+strconv.Itoa(n)+" on this machine", false)
		a.displayChoice[n] = c
		a.wire(c, call{action: panel.ActionSetDisplay, display: n})
	}

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
func (a *app) run(c call) {
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

// visible is the inputs whose rows are shown, which is also what the menu is
// shaped from.
func visible(inputs []cli.Input) []cli.Input {
	out := make([]cli.Input, 0, len(inputs))
	for _, in := range inputs {
		if !in.Hidden {
			out = append(out, in)
		}
	}
	return out
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
