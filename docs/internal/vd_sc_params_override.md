The annotations below are read from the `StorageClass` and override the volume and access modes
that would otherwise be resolved from the `StorageProfile` of that `StorageClass` when a
`PersistentVolumeClaim` is created for a VD. Resolution order: `StorageClass` annotations, then
the `StorageProfile` status; a mode set by an annotation wins, and any mode left unset falls back
to the profile.

| Annotation                                          | Valid values                     |
| --------------------------------------------------- | -------------------------------- |
| virtualdisk.virtualization.deckhouse.io/volume-mode | `Block`, `Filesystem`            |
| virtualdisk.virtualization.deckhouse.io/access-mode | `ReadWriteOnce`, `ReadWriteMany` |

The same annotations placed on a VD or a VI itself are ignored: only the `StorageClass` carries them.
