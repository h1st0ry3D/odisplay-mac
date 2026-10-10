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

### 1. odisplay-cli

This is required. Without it the app starts and says so, listing where it looked.

```sh
git clone https://github.com/h1st0ry3D/odisplay-cli
cd odisplay-cli
go build -trimpath -o ~/.local/bin/odisplay ./cmd/odisplay
```

And the programs it drives:

```sh
brew install ddcctl                                        # the monitor
git clone https://github.com/marcocosta97/mxswitch && cd mxswitch && make && make install PREFIX="$HOME/.local"
```

### 2. This app

```sh
git clone https://github.com/h1st0ry3D/odisplay-bar
cd odisplay-bar
./build/build.sh
```

That produces `build/OdisplayBar.app`. Move it to `/Applications` and open it.

A bundle rather than a bare binary, because macOS gives a menu bar app a Dock icon and no stable
identity without one, and TCC permissions are granted per bundle.

### 3. Permissions

Moving the keyboard and mouse sends HID reports, which macOS gates behind **Input Monitoring**.
If it does not move, grant it to Odisplay in System Settings → Privacy & Security → Input
Monitoring.

Keep the `codesign -s -` in the build script. macOS identifies an unsigned binary by path *and*
code hash, so an unsigned rebuild silently loses the permission it had.

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