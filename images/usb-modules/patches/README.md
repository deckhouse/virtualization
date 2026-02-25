
This directory contains patches used to build the following out-of-tree kernel modules:

- `usbip-core`
- `usbip-host`
- `vhci-hcd`

---

## 001-vhci-increase-ports-and-controllers.patch

This patch modifies the default configuration of the `vhci-hcd` (Virtual Host Controller Interface) driver.

### Changes

- Sets the number of ports per virtual hub to **16** (hardcoded).
- Sets the number of virtual host controllers to **4** (hardcoded).
- Removes dependency on:
  - `CONFIG_USBIP_VHCI_HC_PORTS`
  - `CONFIG_USBIP_VHCI_NR_HCS`

### Resulting Capacity

Each VHCI controller provides:
- 2 hubs (USB 2.0 and USB 3.0)
- 16 ports per hub

With 4 controllers total:

4 controllers × 2 hubs × 16 ports = **128 ports**

This allows up to **128 USB devices** to be attached simultaneously via USB/IP (subject to kernel and system limitations).

> Note: The number of ports and controllers is now fixed at compile time and no longer configurable via kernel config options.
