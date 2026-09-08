# SPICE display: annotations and fixed tuning

What a user gets is one field, `spec.spice.enabled` of the `VirtualMachine`. Everything
else described here is internal: annotations the platform writes for itself, and the SPICE
parameters that are compiled in rather than exposed. None of it belongs in the user
documentation — it is written down here so that the next person does not have to read the
converter of the fork to find out why the server behaves the way it does.

## Annotations

### `internal.virtualization.deckhouse.io/spice`

How `spec.spice.enabled` reaches the domain. The platform sets it on the VMI template of
the KVVM (`kvbuilder.SetSpiceDevices`), KubeVirt copies it onto the `VirtualMachineInstance`,
and the fork reads it in two places:

- `pkg/virt-launcher/virtwrap/converter/converter.go` — whether to add the second
  `<graphics>`, the vdagent channel and to route the USB redirection slots through SPICE;
- `pkg/virt-controller/services/renderresources.go` — the memory overhead of the launcher pod.

Not a knob. It is set from the spec on every reconcile and removed when SPICE is turned off,
so a value written by hand on a `VirtualMachine` is overwritten or dropped — `SetSpiceDevices`
deliberately runs after `SetMetadata`, which is what copies user annotations onto the template.

### `virtualization.deckhouse.io/video`

Overrides the model of the video adapter: `virtio`, `bochs`, `vga`, `ramfb`. Independent of
SPICE — virtio-gpu is worth having on plain VNC too, it gives damage rectangles, a hardware
cursor and resize. Old guests (Windows 7, XP) have no driver for it and are better off on the
default, hence the explicit opt-in.

Enabling SPICE sets `virtio` on its own; this annotation still wins, because `SetVideoModel`
runs right after `SetSpiceDevices`.

## Devices attached together with SPICE

Turning SPICE on attaches, besides the display itself: a virtio-gpu video adapter, an ich9
sound card and four USB redirection slots (`ClientPassthrough`). Turning it off detaches all
of them. QXL is not used: it is the only model that ever sent drawing commands to SPICE, but
its Windows driver was dropped at Windows 10, so on modern guests it degrades to a plain VGA.

## Tuning that is fixed, not configurable

The `<graphics type="spice">` element is built with constant parameters
(`converter.go`, next to the VNC graphics):

| Parameter | Value | Why |
|---|---|---|
| `image compression` | `auto_glz` | Lossless codec for the general case. |
| `jpeg compression` | `always` | Lossy compression of photo-like areas. |
| `zlib compression` | `always` | Deflates what is left after the image codec. |
| `streaming mode` | `filter` | Video-area detection stays on without forcing a stream for every update. |
| `playback compression` | `on` | Opus instead of raw audio on the playback channel. |

They were annotations during development and are constants now. The reason is that in this
topology the client always arrives over a unix socket behind the proxies of the platform, so
the server never sees a WAN connection and every "auto" heuristic inside SPICE stays dormant:
there is nothing for a per-VM knob to choose between. `lz4` is not on the list because
libvirt rejects the domain with it, even though libspice-server is built with it.

The one thing a person can still change per connection is the image codec the client asks
for: `d8 v spice --preferred-compression=…`. It is a client-side request, needs no restart
and is meant for comparing codecs on a live VM.

## Memory overhead

The launcher pod gets `35Mi` statically and `44Mi` for one session when the annotation is
present (`SpiceStaticOverhead`, `SpiceSessionOverhead` in `pkg/virt-controller/services/template.go`
of the fork). The reservation does not depend on anyone being connected: with Guaranteed QoS
the request equals the limit, so an underestimate is not swapping but an OOM kill of the pod
together with the VM.

Measured on live guests at 1280x800: with no client attached, +28 MiB on Windows 11 with
4 GiB of RAM and +15 MiB on a Linux guest with 1 GiB; a connected client adds 10-13 MiB
more. Both constants therefore have room — the static one follows the worse of the two
guests, the session one sits far above a single client, and a single client is all there
can be: `acquireSpiceSession` in `pkg/virt-handler/rest/console.go` of the fork evicts the
previous one.

**These numbers hold for one resolution only.** Reading the server code suggests the static
part does not scale with the resolution while the session part follows the changed area
rather than the pixel count (the GLZ dictionary is sized once per channel, the image cache
is a 1024-entry table), but nobody has measured the same guest at two resolutions. A
`resolution` field in the API would turn both constants into functions of it, and the
measurement has to come first: same VM, same scene, RSS of qemu minus its guest mappings,
read after the process settles (a fresh one is 15-20 MiB low).

The field is planned as `spec.spice.resolution` with named presets — `HD`, `FHD`, `QHD`,
`UHD` — so a user picks a screen size rather than counting megabytes. The video device is
shared with VNC, so the setting decides what every client sees, not only a SPICE one.

Video memory is the ceiling, and it is a constant today: the converter pins `VRam` at
16384 KiB with one head. The value comes from upstream KubeVirt and the fork does not
change it. At 32 bits per pixel:

| Preset | Pixels | Framebuffer | Fits 16 MiB |
|---|---|---|---|
| `HD` | 1280x720 | 3.5 MiB | yes |
| `FHD` | 1920x1080 | 8 MiB | yes |
| `QHD` | 2560x1440 | 14 MiB | only just |
| `UHD` | 3840x2160 | 32 MiB | no, vram has to grow |

So the knob is two changes at once: the preset the user picks, and vram derived from it
instead of the hardcoded 16384.

## Patches behind the display

- `images/qemu/patches/003-vnc-jpeg-defaults.patch` — JPEG in the VNC server.
- `images/qemu/patches/004-spice-video-codec.patch` — the codec list QEMU accepts.
- `images/qemu/patches/005-vnc-no-gradient-filter.patch` — no gradient-filtered Tight rectangles;
  noVNC cannot decode them, and they appeared together with lossy encodings.

There are no patches aimed at the video stream any more: the three that were
(`006-spice-stable-update-grid.patch`, `007-spice-merge-update-columns.patch` and the
spice-server `001-stream-detection-tolerance.patch`) were dropped on 2026-08-31 after measurement showed the stream
covers about 5% of a screensaver session and is never created on a real video, while SPICE costs
more than VNC with JPEG on the same scene either way.
