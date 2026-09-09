Аннотации ниже читаются с `StorageClass` и переопределяют режимы тома и доступа, которые иначе
были бы определены по `StorageProfile` этого `StorageClass` при создании `PersistentVolumeClaim`
для VD. Порядок определения: аннотации `StorageClass`, затем статус `StorageProfile`; режим,
заданный аннотацией, имеет приоритет, а незаданный режим берется из профиля.

| Аннотация                                           | Допустимые значения              |
| --------------------------------------------------- | -------------------------------- |
| virtualdisk.virtualization.deckhouse.io/volume-mode | `Block`, `Filesystem`            |
| virtualdisk.virtualization.deckhouse.io/access-mode | `ReadWriteOnce`, `ReadWriteMany` |

Те же аннотации, установленные на самом VD или VI, игнорируются: их несет только `StorageClass`.
