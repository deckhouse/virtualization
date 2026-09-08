# Patches

This directory contains downstream patches applied to the QEMU source during the image build.
Patch files are applied in lexicographical order.

The `seabios/` subdirectory contains firmware patches that are applied separately before the QEMU build.
Its behavior is documented in `images/qemu/patches/seabios/README.md`.

## 001-revert-scsi-disk-serial-truncate.patch

Reverts upstream commit
[`75997e182b69`](https://github.com/qemu/qemu/commit/75997e182b695f2e3f0a2d649734952af5caf3ee),
which started rejecting SCSI disk `serial` values that exceed the internal length limits.

Why this patch is kept:

- Older VM definitions relied on the historical QEMU behavior where long serials were accepted.
- The guest-visible value was truncated, but the VM still booted successfully.
- Strict validation turns the same configuration into a startup error and breaks upgrades.

Effect:

- Long `serial` values are accepted again.
- Legacy truncation behavior is preserved instead of failing device initialization.

## 002-no-bootable-qmp.patch

Adds a `NO_BOOTABLE_DEVICE` QMP event that is emitted when `isa-debugcon` device receives the exact
string `No bootable device.` in the debug output stream.

Why this patch is kept:

- Management components can detect a boot failure through QMP instead of parsing debug logs.
- The event provides a stable signal that can be consumed by automation.
- It is intended to work together with firmware changes that output the marker string to the
  debug port.

Effect:

- `isa-debugcon` gets a new `watch-no-bootable=on` property.
- When enabled, QEMU watches the debug console output and emits `NO_BOOTABLE_DEVICE` after the
  full marker string is received.
- The patch also adds a qtest that verifies the event is generated.

## 003-vnc-jpeg-defaults.patch

Makes the Tight encoder actually reach for JPEG on a console that goes through several hops.

Why this patch is kept:

- The server starts every client at `quality = -1` (lossless). gtk-vnc, which backs `remote-viewer`,
  does not send a QualityLevel pseudo-encoding unless lossy encoding is enabled, and even then only
  offers level 5, so clients stay on near-raw tiles.
- Upstream treats levels 5 to 7 as lossless and guards JPEG behind an update-frequency threshold that
  plain window dragging never reaches, so JPEG never engages at those levels.

Effect:

- The default quality level becomes 6. An explicit level from the client still wins.
- Levels 5 to 7 encode as JPEG at quality 50/60/70; levels 8 and 9 keep the upstream lossless meaning.
- Measured on a 1280x800 desktop: 64 KiB per frame at 141 fps against 719 KiB at 38 fps.
- Needs `--enable-vnc-jpeg` in the build and the libvirt `lossy=on` patch; without either the code
  path stays dormant.

## 004-spice-video-codec.patch

Exposes a `video-codec` parameter on the `-spice` command line, e.g.
`video-codec=gstreamer:vp8;spice:mjpeg`.

Why this patch is kept:

- `libspice-server` has had `spice_server_set_video_codecs()` since 0.13.2 and is linked against
  gstreamer, but upstream QEMU never exposed the setter.
- Without it the server is stuck with its built-in MJPEG encoder for streamed areas, no matter which
  gstreamer plugins the image ships.

Effect:

- The encoder list for streamed areas becomes configurable; the client negotiates it and skips any
  encoder it cannot decode.
- An invalid list fails the domain startup instead of being ignored.
- Needs the libvirt patch `007-spice-video-codec` to pass the option through, and a stream has to be
  detected in the first place: the server only creates one where the same rectangle keeps being
  redrawn.

## 005-vnc-no-gradient-filter.patch

Stops the Tight encoder from ever emitting a gradient-filtered rect.

Why this patch is kept:

- noVNC, which the web console is built on, does not implement the gradient filter. The first such
  rect makes it throw `Error decoding rect: Error: Gradient filter not implemented` and drop the
  websocket, so the console reconnects in a loop and never shows a picture.
- The filter only becomes reachable once `lossy=on` is set: `tight_detect_smooth_image()` returns 0
  immediately while `vd->lossy` is false. That is why the console worked before `lossy=on` was
  introduced for JPEG by `003-vnc-jpeg-defaults` and the libvirt `006-vnc-lossy-encoding` patch, and
  broke together with them.
- A gradient rect needs a smooth, rarely updated area of the screen. A photo wallpaper on a Windows
  desktop is enough; a text console never produces one, which is why Linux guests were unaffected.

Effect:

- Smooth areas that would have been gradient-filtered are sent as plain full-colour data, which every
  client decodes. Slightly more bytes for those rects, no protocol risk.
- The JPEG path is untouched: frequently updated smooth areas are handled by `send_sub_rect_jpeg` and
  never reach this code.

This patch is permanent, not a stopgap. Teaching noVNC the filter would only fix the console shipped
with the same release: the VNC of a virtual machine is also opened by clients we do not ship and do
not control, and every one of them that lacks the filter would break the same way. The bytes the
patch costs buy compatibility with all of them.

## Dropped: 006-spice-stable-update-grid, 007-spice-merge-update-columns

Both patches shaped the update rectangles QEMU hands to SPICE so that the server would start a video
stream and keep it. They were removed on 2026-08-31, together with the spice-server patch
`001-stream-detection-tolerance`, because measurement did not support the goal:

- On a real video in a browser — the friendliest scene a video codec can get — no stream was created
  at all in either of two runs, and SPICE cost 2362-2608 KiB/s against 1744-1798 KiB/s for VNC with
  JPEG on the same scene.
- On a screensaver the streams do appear, and `007` did lengthen them (a stream lived 0.54-1.35 s
  instead of 0.1-0.7 s), but they still covered only about 5% of the session. The other 95% went as
  ordinary drawables.

Keeping three downstream patches to move a metric from 1% to 5%, on a path that loses to VNC anyway,
is not worth the maintenance. `004-spice-video-codec` stays for now: it only declares which codecs
the server may pick, and costs nothing when no stream is created.

