# Migrations

## qemu-max-length-36

Fix disk serial handling for successful migration:

Previously, we set the disk serial to match the disk name, relying on Kubernetes to ensure the uniqueness of names. 
However, after this [commit](https://github.com/qemu/qemu/commit/75997e182b695f2e3f0a2d649734952af5caf3ee) on QEMU, we can no longer do this, as QEMU no longer truncates the serial to 36 characters and now returns an error.
We must handle the truncation ourselves. 
However, truncation does not guarantee uniqueness, so we switched to generating an MD5 hash of the disk UID. 
The MD5 hash is easy to reproduce and fits the required 32-character length.

To ensure a successful migration, existing `kvvm` resource need to be patched to reflect the new serial format.
