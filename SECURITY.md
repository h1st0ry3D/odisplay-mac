# What the two helper programs do

macOS asks you to grant **Input Monitoring** to something before it will let that thing read
keystrokes from a HID device. That is a serious permission, so this is a read of the source of both
programs `odisplay` drives, and what each one actually needs.

Nothing here is a substitute for reading the code yourself. Both are small enough to read in one
sitting, which is the main reason they were chosen.

## Summary

| | Needs Input Monitoring? | Why |
|---|---|---|
| `ddcctl` | **No** | Talks I2C to the display over IOKit. Never opens a HID device. |
| `mxswitch` | **Yes** | Opens the keyboard/mouse HID interface to send one HID++ command. |

**Grant Input Monitoring to the app that needs it, not to everything.** If `odisplay-mac` is what
you run, `mxswitch` is a child of it, and macOS attributes the permission to the binary that opens
the device. The app's `Info.plist` includes the usage string for that reason.

## ddcctl

**Source:** [kfix/ddcctl](https://github.com/kfix/ddcctl), GPL-3.0, `src/ddcctl.m` (488 lines) and
`src/DDC.c` (469 lines).

```
$ grep -c 'IOHID' src/ddcctl.m src/DDC.c
src/ddcctl.m:0
src/DDC.c:0
```

It never touches the keyboard or mouse. What it does use:

- `IOKit/IOKitLib.h` and `IOKit/graphics/IOGraphicsLib.h`: find the display's framebuffer
- `IOI2CInterface`: send DDC/CI requests to the monitor over the video cable
- `AppKit/NSScreen.h`: enumerate displays

No sockets, no `popen`, no `system`, no `exec`, no file writes. The only data it sends is the DDC
command you asked for, down the display cable.

**Established:** 1958 stars, first commit 2014, GPL-3.0. Last commit November 2022, and the project
describes itself as being in maintenance mode. For something that talks to hardware over a frozen
interface, that is reasonable rather than abandoned.

**Note:** it does exclude DPMS toggling. The source has a comment that the `-p` flag used to brick
monitors and is now a no-op
([issue #89](https://github.com/kfix/ddcctl/issues/89)). Only the input-source path matters here,
and that one is intact.

**Does it need any permission at all?** Its source requests no TCC-gated API: no accessibility, no
screen recording, no HID. Whether it needs `sudo` depends on your Mac and port, but that is an
IOKit access question, not a privacy prompt.

## mxswitch

**Source:** [marcocosta97/mxswitch](https://github.com/marcocosta97/mxswitch), MIT,
`macos/mxswitch.c` (424 lines, one file, no dependencies).

**This is the one that needs Input Monitoring.** It opens the Logitech HID interface to write one
HID++ command.

### What it sends

Feature `0x1814` (`ChangeHost`), function 1 (`setCurrentHost`), with a one-byte parameter: the
channel number minus one.

```
static const uint8_t probe[3] = { FEAT_CHANGE_HOST >> 8, FEAT_CHANGE_HOST & 0xFF, 0 };
...
if (reply[4] == 0) continue;     /* feature not supported */
t->feature_idx = reply[4];
...
uint8_t host = (uint8_t)(channel - 1);
hidpp_call(t.dev, t.report_id, t.dev_idx, t.feature_idx, 1, &host, 1, NULL, 0);
```

That is 7 or 20 bytes out, depending on the report length. It queries the root feature at runtime
to find where `0x1814` sits in that device's table rather than hardcoding an offset, which is the
correct way and is what its README claims. The write is fire-and-forget, which is also correct: by
the time the device would reply it has already left this machine.

### What it touches

- **Network: none.** No `socket`, `connect`, `curl`, `NSURL`, or any HTTP symbol in the file.
- **Files written: none.**
- **Persistence: none.** It reads no config and saves no state.

Two shell invocations, both benign, both worth naming because they are the only `popen`/`system`
calls in it:

1. A read-only `SELECT` against the TCC database, to answer "is Input Monitoring already granted to
   *this* binary":

   ```
   /usr/bin/sqlite3 "/Library/Application Support/com.apple.TCC/TCC.db" \
     "SELECT auth_value FROM access WHERE service='kTCCServiceListenEvent' AND client='<path>' LIMIT 1;"
   ```

   Read-only. It exists because `CGPreflightListenEventAccess()` returns true if any parent (for
   example, your terminal) already has the grant, which would make `--setup` skip a prompt you
   still need. On a normal run the database is unreadable under SIP, and it falls back to the
   preflight call.

2. `/usr/bin/open 'x-apple.systempreferences:...'`: opens the Input Monitoring pane for you.

### Concerns worth stating

**The project is new.** Created 2026-07-28, 3 stars, 2 forks, one author. Everything about the
*code* checks out, but the code is not the only thing you are trusting: you are trusting that the
binary you build is the source you read, and that the author has no reason to change it later.
`ddcctl` has had 12 years and 1958 stars for people to notice something.

Mitigations that are in your hands: build it yourself from the source you read rather than using a
binary someone else built, and pin the commit. `git clone` then `git checkout <sha>` before `make`.

**It uses `popen` and `system`, which fork a shell.** The interpolated value is the binary's own
path from `_NSGetExecutablePath`, and single quotes are escaped for the SQL string, but the path also
lands inside a double-quoted shell context, where `$`, backticks and backslashes would be
expanded. A path containing those would misbehave. You control that path, so this is a hardening
nit rather than a hole, and it is the one thing in either program I would fix.

## What was checked

Both files were read in full, and searched for:

- network calls: `socket`, `connect`, `curl`, `NSURL`, `CFURL`, `http`
- process spawning: `popen`, `system`, `exec*`, `fork`
- file writes: `fopen`, `fwrite`, `unlink`, `rename`, `chmod`
- TCC-gated APIs: `CGPreflightListenEventAccess`, `CGRequestListenEventAccess`,
  `AXIsProcessTrusted`, `CGEventTap`, `ScreenCaptureKit`
- hidden payloads: long base64 or hex string literals

`mxswitch.c` had no base64 or hex blob, no network call, and no file write. `ddcctl` had no
TCC-gated API and no HID access at all.

## If you would rather not

You do not have to grant Input Monitoring to use the display half. Only the Easy-Switch feature
needs it. Set every input's channel to `off` and `odisplay-mac` will switch the monitor and leave
the keyboard and mouse alone: no HID access, no prompt.

If you want the devices to follow but not with `mxswitch`, the alternative is Logi's own Options+,
which is a much larger and closed program that you would be trusting instead. You could also write
your own using the protocol above; it is one HID++ call.

## For reference

The HID++ `ChangeHost` protocol is documented in
[Solaar's source](https://github.com/pwr-Solaar/Solaar) and in Logitech's HID++ 2.0 specification.
`odisplay` on Linux uses Solaar for the same command, and `mxswitch` is the macOS equivalent of
that one feature.
