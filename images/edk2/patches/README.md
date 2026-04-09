# 001-isa-debug-port-no-bootable-device-message.patch
If an EFI “No bootable device” error occurs, or the system drops into the EFI shell, output “No bootable device.” to the ISA debug port.