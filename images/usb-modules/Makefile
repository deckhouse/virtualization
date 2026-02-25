# SPDX-License-Identifier: GPL-2.0
# Out-of-tree build: no kernel .config needed.
# Use: make -C /lib/modules/$(uname -r)/build M=$(pwd) modules

obj-m += usbip-core.o
usbip-core-y := usbip_common.o usbip_event.o

obj-m += vhci-hcd.o
vhci-hcd-y := vhci_sysfs.o vhci_tx.o vhci_rx.o vhci_hcd.o

obj-m += usbip-host.o
usbip-host-y := stub_dev.o stub_main.o stub_rx.o stub_tx.o
