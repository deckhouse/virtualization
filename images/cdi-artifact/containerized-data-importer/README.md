# Virtualization CDI Importer Runtime

This directory contains the reduced importer runtime used by the Deckhouse virtualization module.
It is based on the KubeVirt Containerized Data Importer codebase and keeps the original Apache 2.0 licensing.

Only the runtime code needed by virtualization importer pods is kept here:

- container image unpack/import into a PVC;
- raw/qcow2 conversion, resize, and progress reporting;
- nbdkit/qemu helpers needed by that import path.

CDI controllers, APIs, DataVolume logic, upload proxy/server, VDDK, imageio, S3, GCS, and upstream development tooling are intentionally removed.
