# Patches

## 001-revert-scsi-disk-serial-truncate.patch

This patch reverts the commit that introduced strict length enforcement for the SCSI disk `serial` property.

### Background

Before the reverted commit, scsi-disk accepted serial numbers of arbitrary length, but the value seen by the guest was silently truncated to 36 characters. While this limitation was arbitrary, it ensured compatibility with existing guest behavior. The change to enforce strict length validation introduced potential compatibility issues, making it impossible to upgrade to newer QEMU versions seamlessly.### Why This Revert Is Necessary

For the time being, we need to maintain backward compatibility until a seamless migration to the new behavior can be implemented. By reverting the commit, we restore the previous behavior where serial numbers longer than 36 characters are truncated instead of causing an error.

### Reverted Commit
[Commit 75997e182b69](https://github.com/qemu/qemu/commit/75997e182b695f2e3f0a2d649734952af5caf3ee)
