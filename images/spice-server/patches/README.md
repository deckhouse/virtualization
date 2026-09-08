# Patches

Downstream patches applied to the libspice-server source during the image build.
Patch files are applied in lexicographical order.

There are none at the moment.

`001-stream-detection-tolerance` lived here until 2026-08-31 and was dropped together
with the two QEMU patches it worked with (`006-spice-stable-update-grid`,
`007-spice-merge-update-columns`). All three aimed at the same thing — getting the
SPICE video stream to start and to stay alive — and measurement said the aim is not
worth the code:

- On a real video playing in a browser the stream was never created at all, across two
  runs, while SPICE spent 2362-2608 KiB/s against 1744-1798 KiB/s for VNC with JPEG on
  the very same scene.
- On a screensaver, where streams do appear, they covered around 5% of the session.
  The remaining 95% went as ordinary drawables, so the codec decided almost nothing.

Since the library is built from source only for the sake of this directory, taking
libspice-server from pm packages is now an option — see the SPICE notes.
