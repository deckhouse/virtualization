# Virtualization PVC Importer Runtime

This directory contains the reduced importer runtime used by the Deckhouse virtualization module.
It is based on the KubeVirt Containerized Data Importer codebase and keeps the original Apache 2.0 licensing.

Only the runtime code needed by virtualization importer pods is kept here:

- container image unpack/import into a PVC (DVCR path uses qemu-img);
- host-assigned PVC-to-PVC clone uses nbdcopy for byte-for-byte copy over NBD;
- raw/qcow2 conversion, resize, and progress reporting;
- nbdkit helpers needed by that import path.

CDI controllers, APIs, DataVolume logic, upload proxy/server, VDDK, imageio, S3, GCS, and upstream development tooling are intentionally removed.

## Where the code came from

Upstream base: CDI `v1.60.3`. The code was copied from the maintenance fork
`fox.flant.com:deckhouse/virtualization/fork/containerized-data-importer`, branch
`v1.60.3-virtualization`. The copy landed in commit `c7ffdbad7` on 2026-07-09, when the tip of
that branch was `6db791a90` (2026-07-06), tag `v1.60.3-v12n.21`.

Do not expect a byte-for-byte match with any fork tag: the copy is both reduced and reworked.
What is ours rather than the fork's:

- `pkg/image/directio.go` and the `convertThreads` parameter of `ConvertToFormatStream`, where
  the fork went the other way with `pkg/util/nfs.go` and a `useDirectIO` flag;
- `pkg/importer/nbdcopy.go`, which the fork does not carry at all;
- whatever the reduction dropped along the way, such as `SetRecommendedLabels` in
  `pkg/util/util.go` and the quota constants of `pkg/common/common.go`.

Note the fork tag here whenever this copy is refreshed - it is the only record of what the copy
should be compared against.
