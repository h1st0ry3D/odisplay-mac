# odisplay-bar

A menu bar app for macOS. Switch the monitor, and take the Logitech keyboard and mouse with you.

It is the second front end for [odisplay-cli](https://github.com/h1st0ry3D/odisplay-cli), the
same one the [Omarchy panel](https://github.com/h1st0ry3D/odisplay) uses.

## What it does

```
Odisplay
──────────────────────────
Sent Mac Mini, devices on host 3.
──────────────────────────
X
  Keyboard and mouse: off  ▸
Mac Mini
  Keyboard and mouse: 3    ▸
PC
  Keyboard and mouse: off  ▸
──────────────────────────
Reload settings
Quit
```

One block per input. The input's own name is the top item, and clicking it switches. The line
under it opens the Easy-Switch channels, with a tick on the one that is in force, so you can see
what a switch will do before you make one.

The top line reports the last switch and is kept in one piece: `Sent Mac Mini, devices on host 3.`
or, when it went wrong, why.

## What it deliberately does not do

It does not decide the order things happen in. That belongs to `odisplay`, and it is the one rule
that must not be written twice:

> The monitor moves first. The keyboard and mouse move only once it has.

Clicking a name is **one call**. This app never sees the individual commands, so it cannot put the
monitor move after the devices move. Clicking a channel only writes a setting; nothing moves until
that input is switched to.

## Install

### What you need first

| | What | Why | Required |
|---|---|---|---|
| CLI | [`odisplay`](https://github.com/h1st0ry3D/odisplay-cli) | Does the switch, and owns the order it happens in | **Yes** |
| macOS | [`ddcctl`](https://github.com/kfix/ddcctl) | Moves the monitor, on a connection that supports DDC | **Yes** |
| macOS | [`mxswitch`](https://github.com/marcocosta97/mxswitch) | Moves the keyboard and mouse | Only for the Easy-Switch channels |
| Build | Go 1.22 or newer | Building both of the above | To build |

Nothing here needs `sudo`. Without Input Monitoring the monitor still switches, and the app says
the devices did not move rather than pretending.

**Check the Mac's connection before you build any of this.** DDC/CI only works on some ports on
Apple Silicon, and if yours does not, nothing above will move the monitor. See
[If the monitor does not switch](#if-the-monitor-does-not-switch).

### 1. The programs it drives

```sh
brew install ddcctl
```

And, for the keyboard and mouse, [mxswitch](https://github.com/marcocosta97/mxswitch), which sends
the HID++ ChangeHost command itself so Logi Options+ does not have to be running:

```sh
git clone https://github.com/marcocosta97/mxswitch
cd mxswitch && make && make install PREFIX="$HOME/.local"
```

`brew` puts ddcctl in `/opt/homebrew/bin` on Apple Silicon and `/usr/local/bin` on Intel. Both are
looked for, so it does not matter which you have.

### 2. odisplay-cli

```sh
git clone https://github.com/h1st0ry3D/odisplay-cli
cd odisplay-cli
go build -trimpath -o ~/.local/bin/odisplay ./cmd/odisplay
```

`/usr/local/bin` works too, though that needs `sudo`. The app looks in `/usr/local/bin` first,
`~/.local/bin` second, then `/opt/homebrew/bin`.

### 3. Check before you trust it

This moves nothing:

```sh
odisplay doctor
```

It reports whether the programs are installed and whether the display number in the settings is one
this machine actually has. **Read it before the first switch** — a display number this Mac will not
accept makes every switch a silent no-op while the keyboard and mouse still move.

### 4. This app

```sh
git clone https://github.com/h1st0ry3D/odisplay-bar
cd odisplay-bar
./build/build.sh
```

That runs the tests first, then builds `build/OdisplayBar.app`.

It is a bundle rather than a bare binary on purpose: macOS gives a menu bar app a Dock icon and no
stable identity without one, and permissions are granted per bundle.

```sh
mv build/OdisplayBar.app /Applications/
open /Applications/OdisplayBar.app
```

The icon appears in the menu bar. Click it for the menu.

### 5. Permissions

Moving the keyboard and mouse sends HID reports, which macOS gates behind **Input Monitoring**.

If a switch moves the monitor but the devices do not, grant it in System Settings → Privacy &
Security → Input Monitoring, adding Odisplay if it is not already listed. Restart the app after.

> Keep the `codesign --sign -` in `build/build.sh`. macOS identifies an unsigned binary by path
> *and* code hash, so an unsigned rebuild silently loses a permission it already had. If you build
> your own, do not drop that step.

### If the monitor does not switch

This is a macOS limitation, not a bug in the app, and it is worth knowing before you spend an
evening on it.

**DDC/CI on Apple Silicon only works on some connections.** macOS reaches a monitor over private
IOKit interfaces that differ by port, and the pattern people report is:

- Thunderbolt and USB-C generally work.
- A **built-in HDMI port on an M-series Mac often does not**, and neither does some USB-C to HDMI
  adapter.
- Intel Macs are less fussy.

So if `odisplay doctor` reports the programs present and the display number is right, but the
monitor never moves, test the connection before anything else:

```sh
ddcctl -m          # does ddcctl see the monitor at all?
```

If that lists nothing, it is the connection. Moving the Mac to a Thunderbolt or USB-C cable is the
usual fix. Nothing in this app or the CLI can work around it, because there is no I2C channel to
talk to.

A switch that cannot move the monitor exits 2 and **leaves the keyboard and mouse alone**, so this
fails safely: you keep your input.

### Uninstall

```sh
rm -rf /Applications/OdisplayBar.app
rm ~/.local/bin/odisplay                       # if you want the CLI gone too
```

Your settings live in `~/.config/odisplay/odisplay.json` and are shared with the CLI, so they
survive. Delete that too if you want them gone.

### Updating

```sh
cd ~/github/odisplay-bar && git pull && ./build/build.sh
mv build/OdisplayBar.app /Applications/
```

Rebuilding replaces the app, so re-check Input Monitoring afterwards if a switch stops moving the
devices.

## Settings

The same file the CLI uses, so both machines can share it:

```
~/.config/odisplay/odisplay.json
```

If the same monitor is driven from an Omarchy box and a Mac, use the per-platform display number so
one file is right on both:

```json
{ "display": 1, "displays": { "linux": 1, "darwin": 4 } }
```

The app reads whatever `odisplay list --json` reports, which already has the number for this
machine resolved. It has no platform rule of its own.

Run `odisplay doctor` first. It touches no hardware and it tells you whether the display number is
one this machine actually has.

## Status

**This has never been run on a Mac.** It is the only part of this work that has not been.

Everything below was tested here, on Linux, against the real `odisplay` binary:

- `internal/cli` — every call the app makes: the argv arrays, the environment, reading
  `list --json`, the exit codes and what each one means. It runs against the real program and a
  scratch settings file, so it is the contract rather than a mock of it.
- `internal/panel` — what the menu says, which item is ticked, what clicking one would do, where
  the separators fall. No drawing, so it is checkable without a screen.
- `internal/` type-checks with `GOOS=darwin`.

What is unverified is the drawing: about 200 lines of systray calls in
`cmd/odisplay-bar/main.go`, and the `.app` bundle. The first build on a Mac is the first real test
of that.

### One difference from the Omarchy panel

`mxswitch`, which moves the devices on macOS, takes no device name. It picks the device itself and
prefers a mouse, and moves one per call. So **the mouse goes first on macOS**, where the panel sends
the mouse last so the pointer is still here if the keyboard move is what failed.

That is a property of the tool rather than of either front end, and splitting them would not fix
it. It is worth knowing before the first real switch.

## Development

```sh
go test ./internal/...   # runs anywhere, including here
./build/build.sh         # on a Mac; runs the tests first, then makes the bundle
```

To look at the menu shape on a machine with no menu bar, build the model and print it. This is what
the panel tests read, and it is how to see what a set of settings produces:

```sh
go run ./cmd/odisplay-menudump
```

## Licence

MIT