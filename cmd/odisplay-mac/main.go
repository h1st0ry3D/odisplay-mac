// Command odisplay-mac puts a monitor input switcher in the macOS menu bar.
//
// It is a front end. Everything that matters happens in odisplay: which
// commands run, and in what order. This program decides how to draw a list and
// nothing else, so it cannot put the monitor move after the keyboard and mouse
// move because it never sees the individual steps.
//
// One switch is one call. Clicking a name asks odisplay to switch; clicking a
// channel asks it to remember a different channel. The second only writes a
// setting, and nothing moves until that input is switched to.
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

// menu is the live state. systray calls its event loop on one goroutine, and a
// switch can take the length of a Solaar timeout, so the two are kept apart.
type app struct {
	mu       sync.Mutex
	client   *cli.Client
	settings cli.Settings
	status   panel.Status
	busy     bool

	// items is what is on screen now, so a rebuild can take the old menu down
	// rather than accumulating items on every reload.
	items []*systray.MenuItem
}

var state = &app{status: panel.Idle}

func onReady() {
	systray.SetTitle("Odisplay")
	systray.SetTooltip("Odisplay — switch the monitor and the keyboard and mouse")

	quit := systray.AddMenuItem("Quit", "Leave odisplay-mac")
	go func() {
		for range quit.ClickedCh {
			log.Println("quitting")
			systray.Quit()
			return
		}
	}()

	state.reload()
}

// reload re-reads the settings and puts the menu back up.
//
// It is called on start, after every change, and by the Reload item, so a hand
// edit of the settings file takes effect without restarting anything.
func (a *app) reload() {
	ctx := context.Background()

	client, err := cli.New()
	if err != nil {
		log.Printf("%v", err)
		a.render()
		return
	}
	a.mu.Lock()
	a.client = client
	a.mu.Unlock()

	settings, err := client.List(ctx)
	if err != nil {
		log.Printf("could not read the settings: %v", err)
		a.setStatus(panel.Status{Text: "Could not read the odisplay settings.", Error: true})
		return
	}

	a.mu.Lock()
	a.settings = settings
	a.mu.Unlock()
	a.render()
}

// searchedPaths is what to tell someone when the binary is not there.
func searchedPaths() []string {
	var out []string
	for _, c := range cli.Candidates() {
		out = append(out, c)
	}
	return out
}

// setStatus records the last result and puts the menu back up with it.
func (a *app) setStatus(s panel.Status) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
	a.render()
}

// render rebuilds the menu.
//
// old is torn down first: systray adds items rather than replacing them, so a
// rebuild without that leaves every previous menu underneath the new one.
func (a *app) render() {
	a.mu.Lock()
	settings, status, busy := a.settings, a.status, a.busy
	haveClient := a.client != nil
	a.mu.Unlock()

	var model *panel.Model
	if !haveClient {
		model = panel.NotInstalled(searchedPaths())
	} else {
		model = panel.Build(settings, status, busy)
	}
	for _, it := range a.items {
		it.Hide()
	}
	a.items = nil

	a.items = append(a.items, systray.AddMenuItem(model.Status.Text, model.Status.Text))
	a.items[len(a.items)-1].Disable()

	for _, it := range model.Items {
		a.addItem(it)
	}

	log.Printf("menu: %s", model.Describe())
}

// addItem puts one model item on screen and wires it to what it does.
func (a *app) addItem(it *panel.Item) {
	if it.SeparatorBefore {
		// systray has no separator setting, so an empty disabled item with a
		// line above it is the same thing a separator is.
		line := systray.AddMenuItem("", "")
		line.Disable()
		a.items = append(a.items, line)
	}

	var item *systray.MenuItem
	if len(it.Children) > 0 {
		item = systray.AddMenuItem(it.Title+"  ▸", it.Tooltip)
	} else {
		item = systray.AddMenuItem(it.Title, it.Tooltip)
	}
	a.items = append(a.items, item)

	if it.Checked {
		item.Check()
	}
	if it.Disabled {
		item.Disable()
	}

	action := it.Action
	key, host := it.Key, it.Host
	if len(it.Children) > 0 {
		for _, child := range it.Children {
			a.addChild(item, child, key)
		}
		return
	}
	if action == panel.ActionNone {
		return
	}

	// Each item gets its own goroutine because a switch can take ten seconds
	// and the menu has to stay responsive to Quit while it does.
	go func() {
		for range item.ClickedCh {
			a.run(action, key, host)
		}
	}()
}

// addChild puts one channel under a host control.
func (a *app) addChild(parent *systray.MenuItem, child *panel.Item, key string) {
	child2 := parent.AddSubMenuItemCheckbox(child.Title, child.Tooltip, child.Checked)
	if child.Disabled {
		child2.Disable()
	}
	host := child.Host
	go func() {
		for range child2.ClickedCh {
			a.run(panel.ActionSetHost, key, host)
		}
	}()
}

// run does one thing, then puts the menu back.
//
// The whole point of busy is that two switches cannot overlap: on a keyboard
// and mouse, an interleaved pair leaves one machine with the pointer and the
// other with nothing.
func (a *app) run(action panel.Action, key, host string) {
	a.mu.Lock()
	if a.busy || a.client == nil {
		a.mu.Unlock()
		return
	}
	a.busy = true
	client := a.client
	a.mu.Unlock()

	a.render()

	ctx := context.Background()
	switch action {
	case panel.ActionSwitch:
		label := key
		if in, ok := inputByKey(a.settings, key); ok {
			label = in.Label()
		}
		log.Printf("switching to %s", label)
		res := client.Switch(ctx, key)
		a.setStatus(panel.ForSwitch(res, label))

	case panel.ActionSetHost:
		log.Printf("setting %s host to %s", key, host)
		res := client.SetHost(ctx, key, host)
		if res.Outcome() == cli.OutcomeOK {
			a.setStatus(panel.ForSetting(res, "Keyboard and mouse for "+label(key, a.settings)+" set to "+panel.HostLabel(host)+"."))
		} else {
			a.setStatus(panel.ForSetting(res, "Keyboard and mouse: "+panel.HostLabel(host)))
		}

	case panel.ActionReload:
		a.setStatus(panel.Idle)
	}

	a.mu.Lock()
	a.busy = false
	a.mu.Unlock()

	// Whatever happened, re-read: a refused change must not stay on screen as
	// though it took.
	a.reload()
}

func inputByKey(s cli.Settings, key string) (cli.Input, bool) {
	for _, in := range s.Inputs {
		if in.Key == key {
			return in, true
		}
	}
	return cli.Input{}, false
}

func label(key string, s cli.Settings) string {
	if in, ok := inputByKey(s, key); ok {
		return in.Label()
	}
	return key
}
