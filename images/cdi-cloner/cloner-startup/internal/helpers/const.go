package helpers

const (
	/*
		IOCTL request code that queries the size of a block device (e.g., disks, partitions) directly from the kernel

			Structure of ioctl Codes:

			Data Type: 0x8008 (upper 16 bits) indicates:
				Direction: Read from the kernel (0x2) (upper 2 bits).
				Size: 0x008 (remaining 14 bits) == 8 bytes, matching the uint64 return type

			Magic Number : 0x12 (the third byte) identifies this as a block device ioctl

			Command Number : 0x72 (the fourth byte) specifies the exact operation (size query)
	*/
	BLKGETSIZE64 = 0x80081272

	/*
		IOCTL request code that queries the size of a block on a device

		0x12 (prefix): This identifies that the command belongs to the block devices ioctl command group.

		0x68 (sub-command): This is a unique identifier within the block device ioctl commands, specifically asking for the logical block size.
	*/
	BLKSSZGET = 0x1268
)
