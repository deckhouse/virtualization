# ADR: ограничение входящих live migrations на target node

## Статус

Предложено.

## Контекст

В модуле virtualization live migration выполняется через KubeVirt `VirtualMachineInstanceMigration`.
Пользовательские и автоматические сценарии миграции в Deckhouse проходят через несколько уровней:

1. `VirtualMachineOperation` (`VMOP`) создаётся пользователем, контроллером эвакуации, workload-updater или другим компонентом.
2. `vmop-migration-controller` создаёт KubeVirt-ресурс `VirtualMachineInstanceMigration`.
3. KubeVirt создаёт target pod для миграции.
4. Kubernetes scheduler назначает target pod на node.
5. KubeVirt выполняет live migration.
6. Контроллеры virtualization синхронизируют статус KubeVirt migration обратно в `VMOP` и `VirtualMachine`.

Сейчас ограничение параллелизма задаётся через KubeVirt `MigrationConfiguration`:

```yaml
parallelMigrationsPerCluster: <N>
parallelOutboundMigrationsPerNode: <N>
```

В KubeVirt нет симметричной настройки:

```yaml
parallelInboundMigrationsPerNode: <N>
```

Из-за этого платформа умеет ограничивать количество исходящих миграций с source node, но не умеет ограничивать количество входящих миграций на target node. На практике несколько VM могут одновременно мигрировать на одну и ту же target node, даже если для source nodes ограничение уже работает.

Требование: контролировать, что входящих миграций на target node не более одной. Остальные миграции должны штатно ожидать свободный inbound slot, а не завершаться ошибкой.

## Новые вводные

В проекте уже есть механизм ожидания динамических параметров миграции:

- `virtualization-controller` вычисляет параметры через `images/virtualization-artifact/pkg/livemigration/migration_configuration.go`;
- `livemigration-controller` патчит `KVVMI.status.migrationState.migrationConfiguration`;
- KubeVirt/virt-launcher ждёт `migrationConfiguration` перед продолжением миграции.

Основная точка подключения:

```text
images/virtualization-artifact/pkg/controller/livemigration/internal/dynamic_settings_handler.go
DynamicSettingsHandler.Handle(ctx, kvvmi)
```

Сейчас handler выставляет:

```go
kvvmi.Status.MigrationState.MigrationConfiguration = conf
```

Эту точку нужно использовать как gate для inbound migration limit: не выставлять `MigrationConfiguration`, пока target node не получила inbound slot.

## Проблема

Ограничение нельзя надёжно реализовать до создания `VirtualMachineInstanceMigration`, потому что target node становится известна только после создания target pod и его назначения scheduler-ом.

Наш контроллер не должен заранее выбирать target node. Иначе ему пришлось бы повторять часть логики Kubernetes scheduler и KubeVirt placement:

- учитывать `nodeSelector` из `VMOP.spec.migrate.nodeSelector`;
- учитывать placement самой `VirtualMachine`;
- учитывать taints/tolerations, affinities, resources, devices, storage constraints;
- учитывать динамические изменения node и pod scheduling state.

Такой подход будет неполным и не даст гарантии, что KubeVirt и scheduler выберут именно ту node, которую предварительно проверил controller.

Также ограничение должно применяться не только к миграциям, созданным через пользовательский `VMOP`, но и к другим источникам миграций:

- eviction;
- node drain;
- workload updater;
- автоматические системные миграции;
- миграции, созданные напрямую через KubeVirt API.

## Решение

Реализовать inbound migration limit в `virtualization-controller` через задержку выдачи `MigrationConfiguration` до получения `Lease` на target node.

Общий flow:

1. `VirtualMachineInstanceMigration` создаётся как сейчас.
2. KubeVirt создаёт target pod.
3. Kubernetes scheduler назначает target pod на target node.
4. `livemigration-controller` определяет target node.
5. Перед patch-ем `KVVMI.status.migrationState.migrationConfiguration` controller пытается получить inbound slot через Kubernetes `Lease`.
6. Если slot получен, controller выставляет `MigrationConfiguration`, и миграция продолжается.
7. Если slot занят, controller не выставляет `MigrationConfiguration`, помечает VMI annotation-ом ожидания и requeue-ит reconcile.
8. KubeVirt patch в месте ожидания `migrationConfiguration` не должен считать timeout, пока VMI явно помечена как ожидающая inbound lease.
9. Lease освобождается при завершении миграции или через stale recovery.

На первом этапе лимит фиксированный:

```text
parallelInboundMigrationsPerNode = 1
```

При этом механизм должен проектироваться как slot-based limiter: один `Lease` соответствует одному inbound slot на target node. Лимит `1` является частным случаем с одним slot.

## Target node

Target node выбирает Kubernetes scheduler, а не `virtualization-controller`.

`livemigration-controller` определяет target node из доступного состояния KubeVirt:

1. `kvvmi.Status.MigrationState.TargetNode`, если поле заполнено;
2. `kvvmi.Status.MigrationState.TargetPod` → `pod.spec.nodeName`, если target pod уже создан и назначен scheduler-ом.

Если target node ещё неизвестна, inbound limiter не должен блокировать миграцию и не должен пытаться выбрать node самостоятельно. Controller ждёт следующего reconcile, когда KubeVirt/scheduler продвинут scheduling target pod.

## Gate через MigrationConfiguration

`MigrationConfiguration` становится точкой допуска миграции к активной фазе.

Логика в `DynamicSettingsHandler.Handle`:

```go
targetNode := resolveTargetNode(kvvmi)
if targetNode == "" {
    return requeue
}

acquired, err := inboundLimiter.TryAcquire(ctx, kvvmi, targetNode, limit)
if err != nil {
    return err
}

if !acquired {
    markInboundSlotWaiting(kvvmi, targetNode)
    return requeue
}

clearInboundSlotWaiting(kvvmi)
kvvmi.Status.MigrationState.MigrationConfiguration = conf
```

Если lease занята, `MigrationConfiguration` не выставляется. Это удерживает миграцию в уже существующем KubeVirt flow ожидания параметров миграции.

## Annotation model

Для явного состояния ожидания используются annotations на VMI:

```yaml
virtualization.deckhouse.io/inbound-migration-slot: waiting
virtualization.deckhouse.io/inbound-migration-target-node: <node>
```

Правила:

- если inbound slot не получен, controller выставляет `inbound-migration-slot=waiting` и target node;
- пока annotation имеет значение `waiting`, `MigrationConfiguration` не выставляется;
- когда lease получена, controller удаляет waiting annotations и выставляет `MigrationConfiguration`;
- отдельное состояние `acquired` не требуется: наличие `MigrationConfiguration` и отсутствие `waiting` достаточно для продолжения миграции.

Эти annotations нужны не только для диагностики, но и для корректной работы timeout-а ожидания migration parameters в KubeVirt.

## Timeout ожидания migration parameters

В KubeVirt есть ожидание `migrationConfiguration`. Если параметры не появились за заданное время, миграция может быть завершена ошибкой.

Для inbound limiter это поведение нужно изменить:

```text
Если MigrationConfiguration == nil
и VMI имеет annotation virtualization.deckhouse.io/inbound-migration-slot=waiting,
то timeout ожидания migration parameters не должен тикать или не должен приводить к failed migration.
```

Иначе вторая и последующие миграции на ту же target node будут падать по timeout, хотя они штатно стоят в очереди на inbound slot.

Если annotation `waiting` отсутствует, существующее поведение timeout-а сохраняется. Это важно, чтобы реальные проблемы с выдачей migration parameters не маскировались inbound limiter-ом.

## Lease model

Рекомендуемая реализация limiter-а — Kubernetes `Lease` из `coordination.k8s.io/v1`.

Один `Lease` представляет один inbound slot target node.

При лимите `1` для target node доступен один slot:

```text
namespace: d8-virtualization
name: inbound-migration-<node-name-hash>-0
holderIdentity: <migration-or-vmi-namespace>/<migration-or-vmi-name>/<uid>
```

При будущем лимите `5` для той же target node доступны пять независимых slots:

```text
inbound-migration-<node-name-hash>-0
inbound-migration-<node-name-hash>-1
inbound-migration-<node-name-hash>-2
inbound-migration-<node-name-hash>-3
inbound-migration-<node-name-hash>-4
```

Правила:

- если slot lease отсутствует, миграция может создать его со своим holder;
- если slot lease уже принадлежит текущей миграции/VMI, reconcile идемпотентно продолжает выполнение и обновляет `renewTime`;
- если slot lease принадлежит другой active migration, slot считается занятым;
- если владелец slot lease отсутствует, terminal или stale, slot можно перехватить через optimistic update;
- если все slots заняты другими active migrations, текущая миграция остаётся в ожидании `MigrationConfiguration`;
- release удаляет только lease, принадлежащий текущей миграции/VMI.

Рекомендуемый объект:

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  namespace: d8-virtualization
  name: inbound-migration-<node-name-hash>-<slot-index>
  labels:
    virtualization.deckhouse.io/component: inbound-migration-limiter
    virtualization.deckhouse.io/target-node-hash: <node-name-hash>
    virtualization.deckhouse.io/slot-index: "<slot-index>"
  annotations:
    virtualization.deckhouse.io/target-node: <target-node>
    virtualization.deckhouse.io/migration-namespace: <migration-namespace>
    virtualization.deckhouse.io/migration-name: <migration-name>
    virtualization.deckhouse.io/migration-uid: <migration-uid>
spec:
  holderIdentity: <migration-namespace>/<migration-name>/<migration-uid>
  leaseDurationSeconds: 300
  acquireTime: <now>
  renewTime: <now>
```

OwnerReference на `VirtualMachineInstanceMigration` добавлять не нужно, потому что migration namespaced, а lease хранится в namespace control plane. Cross-namespace owner reference для namespaced объектов некорректен.

## TryAcquire

`TryAcquire(ctx, kvvmi, targetNode, limit)` должен работать так:

1. Построить список lease names по `targetNode` и текущему лимиту.
2. Сначала найти lease, который уже принадлежит текущей migration/VMI.
3. Если такой lease найден, обновить `renewTime` и вернуть `true`.
4. Если текущая migration ещё не владеет slot-ом, пройти по всем slots и попытаться занять первый доступный.
5. Если lease не найден, создать lease с holder текущей migration/VMI.
6. Если create завершился conflict/already exists, перечитать slot или перейти к следующему.
7. Если lease принадлежит другой migration, проверить владельца.
8. Если владелец существует и не terminal, считать slot занятым.
9. Если владелец отсутствует или terminal, перехватить slot через `Update` с текущим `resourceVersion`.
10. Если один из slots успешно создан или перехвачен, вернуть `true`.
11. Если все slots заняты активными владельцами, вернуть `false`.

Операции `Get/Create/Update/Delete` для Lease желательно выполнять через non-cached client или APIReader, если это доступно в месте интеграции. Корректность должна опираться на optimistic concurrency Kubernetes API.

## Release и stale recovery

Lease должен освобождаться при завершении миграции:

- `VirtualMachineInstanceMigration` перешла в terminal phase;
- migration завершилась `Completed`/`Failed` на уровне отслеживаемого состояния;
- VMI больше не находится в live migration state;
- владелец lease удалён или его UID не совпадает с UID в annotations.

`Release(ctx, owner, targetNode)` должен быть идемпотентным:

1. Найти lease, принадлежащий текущей migration/VMI.
2. Если lease отсутствует, завершиться успешно.
3. Если lease принадлежит текущему owner, удалить lease.
4. Если delete получил `NotFound`, завершиться успешно.

Если лимит был уменьшен после того, как migration заняла slot с индексом за пределами нового лимита, release всё равно должен уметь найти и удалить её lease. Для этого release может дополнительно list-ить leases по labels `component=inbound-migration-limiter` и `target-node-hash=<hash>`, а затем фильтровать holder текущей migration/VMI.

Stale recovery выполняется при `TryAcquire`:

1. прочитать holder из annotations/`holderIdentity`;
2. найти соответствующую `VirtualMachineInstanceMigration` или VMI;
3. если owner отсутствует, UID отличается или migration terminal, считать lease stale;
4. перехватить lease через optimistic update с `resourceVersion`.

`leaseDurationSeconds` и `renewTime` используются как диагностическая и safety-информация. Нельзя освобождать lease только по истечению времени, если migration-владелец всё ещё существует и не terminal: долгие live migrations допустимы.

## Статусы и условия

Ожидающая inbound slot миграция не должна считаться failed.

На уровне KubeVirt migration и VMI желательно отражать ожидание через annotation:

```text
virtualization.deckhouse.io/inbound-migration-slot=waiting
virtualization.deckhouse.io/inbound-migration-target-node=<node>
```

На уровне `VirtualMachineOperation` можно использовать существующий pending mapping:

```text
VMOP.status.phase: Pending
Completed condition:
  status: False
  reason: MigrationPending
  message: The VirtualMachineOperation for migrating the virtual machine has been queued. Waiting for the queue to be processed and for this operation to be executed.
```

Для лучшей диагностики можно уточнить message, если VMI помечена как ожидающая inbound slot. Добавление нового API reason возможно позже, но для первого этапа не обязательно.

## Конфигурация

### Первый этап

Лимит фиксированный:

```text
parallelInboundMigrationsPerNode = 1
```

Преимущества:

- минимальные изменения публичного API;
- не требует новых ModuleConfig параметров;
- закрывает исходное требование;
- оставляет простой путь к будущему конфигурируемому лимиту.

### Возможное развитие

Позже можно сделать настройку конфигурируемой через ModuleConfig annotation и Helm values:

```yaml
virtualization.deckhouse.io/parallel-inbound-migrations-per-node: "5"
```

Внутренний values path:

```text
virtualization.internal.virtConfig.parallelInboundMigrationsPerNode
```

Так как upstream KubeVirt `MigrationConfiguration` не содержит такого поля, эта настройка будет Deckhouse-specific и должна применяться в логике `virtualization-controller`, а не как поле upstream `MigrationConfiguration`.

## RBAC

`virtualization-controller` должен получить права на leases в namespace `d8-virtualization`:

```text
apiGroups: ["coordination.k8s.io"]
resources: ["leases"]
verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

`list/watch` нужны для cleanup/stale recovery и для случаев, когда release должен найти slot за пределами текущего лимита.

## Изменяемые компоненты

Основные места реализации:

- `images/virtualization-artifact/pkg/controller/livemigration/internal/dynamic_settings_handler.go` — gate перед выставлением `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/livemigration/live_migration_reconciler.go` — release/stale cleanup на завершении миграции;
- `images/virtualization-artifact/pkg/livemigration/migration_configuration.go` — существующая генерация `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/vmop/migration/internal/handler/lifecycle.go` — отображение ожидания в статус `VMOP`, если потребуется;
- KubeVirt patch в месте ожидания `migrationConfiguration` — исключить период `inbound-migration-slot=waiting` из timeout-а ожидания parameters;
- `images/hooks/pkg/hooks/migration-config/hook.go` и `templates/kubevirt/_kubevirt_helpers.tpl` — существующие места конфигурации миграций, без добавления upstream-поля `parallelInboundMigrationsPerNode`.

## Альтернативы

### Альтернатива 1: предварительно выбирать target node в `vmop-migration-controller`

Суть: до создания `VirtualMachineInstanceMigration` выбрать target node и не создавать migration, если node занята.

Недостатки:

- target node должен выбирать Kubernetes scheduler;
- controller должен повторить scheduler logic;
- нет гарантии, что KubeVirt выберет проверенную node;
- не покрывает миграции, созданные не через `VMOP`;
- возможны гонки между несколькими VMOP.

Решение отклонено.

### Альтернатива 2: limiter внутри patched KubeVirt `virt-controller`

Суть: встроить Lease limiter непосредственно в KubeVirt migration control loop.

Преимущества:

- близко к месту управления lifecycle KubeVirt migration;
- можно блокировать продвижение фаз напрямую.

Недостатки:

- больше patch surface в KubeVirt;
- сложнее поддерживать при обновлениях upstream;
- в проекте уже есть Deckhouse-specific gate ожидания `MigrationConfiguration`, который решает задачу меньшим изменением.

Решение отклонено в пользу gate через `MigrationConfiguration`.

### Альтернатива 3: простой подсчёт активных миграций без Lease

Суть: перед выдачей `MigrationConfiguration` list-ить все migrations и считать active incoming на target node.

Преимущества:

- проще реализации;
- не требует дополнительных ресурсов.

Недостатки:

- нет строгой гарантии при concurrent reconcile;
- возможны race conditions;
- поведение зависит от cache freshness.

Можно использовать как дополнительную диагностику, но не как основной механизм гарантии.

Решение отклонено как основной вариант.

### Альтернатива 4: mutating webhook и init container gate

Суть: модифицировать target pod через webhook и удерживать его через init container до получения inbound slot.

Недостатки:

- сложнее операционно;
- требует вмешательства в pod lifecycle;
- уже есть более подходящая точка ожидания `MigrationConfiguration`;
- возможны побочные эффекты для KubeVirt target pod lifecycle.

Решение отклонено.

### Альтернатива 5: reactive abort/retry

Суть: разрешить KubeVirt начать миграцию, а при превышении inbound limit abort-ить или retry-ить лишние миграции.

Недостатки:

- на короткое время лимит может быть превышен;
- создаёт лишние abort/retry циклы;
- хуже пользовательский опыт;
- сложнее отличать штатную очередь от ошибки.

Решение отклонено.

## Последствия

### Положительные

- На target node будет не более настроенного числа активных входящих live migrations; на первом этапе — не более одной.
- Target node по-прежнему выбирает Kubernetes scheduler.
- Не нужно реализовывать собственную scheduler logic в `virtualization-controller`.
- Остальные миграции будут ждать, а не падать.
- Ожидание использует уже существующий gate `MigrationConfiguration`.
- Ограничение будет работать независимо от источника миграции.
- Lease даёт строгую защиту от race condition между несколькими reconcile workers.
- Снижается риск перегрузки target node сетью, CPU, памятью и storage attach операциями.

### Отрицательные

- Требуется новый служебный ресурс `Lease` и логика очистки stale leases.
- Требуется patch KubeVirt timeout-а ожидания `migrationConfiguration`.
- Появляется Deckhouse-specific поведение, которое нужно учитывать при обновлении KubeVirt.
- Возможна меньшая скорость массовой эвакуации, если много VM мигрируют на одну target node.
- Нужно аккуратно синхронизировать annotations, lease ownership и status patch VMI.

## План реализации

### Шаг 1. Добавить inbound limiter в `virtualization-controller`

Добавить компонент примерно такого вида:

```go
type InboundMigrationLimiter interface {
    TryAcquire(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, targetNode string, limit int) (bool, error)
    Release(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance, targetNode string, limit int) error
}
```

Реализация должна использовать `coordination.k8s.io/v1 Lease`.

### Шаг 2. Интегрировать limiter в `DynamicSettingsHandler.Handle`

Логика:

1. определить target node из `kvvmi.Status.MigrationState.TargetNode` или target pod `spec.nodeName`;
2. если target node неизвестна, requeue без выбора node;
3. вызвать `TryAcquire`;
4. если slot не получен, выставить waiting annotations и не выставлять `MigrationConfiguration`;
5. если slot получен, удалить waiting annotations и выставить `MigrationConfiguration`.

### Шаг 3. Изменить KubeVirt timeout ожидания parameters

В KubeVirt patch-е ожидания `migrationConfiguration` добавить правило:

```text
VMI with virtualization.deckhouse.io/inbound-migration-slot=waiting is waiting for inbound lease and must not fail by migration parameters timeout.
```

### Шаг 4. Добавить release/stale recovery

Логика:

1. при terminal migration удалить lease owner-а;
2. при failed/completed состояниях удалить lease;
3. при обнаружении stale holder-а во время `TryAcquire` перехватить slot;
4. сделать release идемпотентным.

### Шаг 5. Синхронизировать диагностику в VMOP

Если VMI имеет `inbound-migration-slot=waiting`, `vmop-migration-controller` должен отображать операцию как pending, а не failed.

Минимальный вариант:

- `VMOP.status.phase = Pending`;
- `Completed.reason = MigrationPending`;
- message содержит информацию про ожидание inbound slot на target node.

### Шаг 6. Тесты

Нужны unit/integration тесты:

1. одна миграция на target node получает slot lease и получает `MigrationConfiguration`;
2. при лимите `1` вторая миграция на ту же target node получает waiting annotations и не получает `MigrationConfiguration`;
3. KubeVirt timeout ожидания parameters не fail-ит VMI с `inbound-migration-slot=waiting`;
4. migration без waiting annotation сохраняет существующее timeout-поведение;
5. миграция на другую target node получает свой lease и продолжается;
6. после завершения первой миграции ожидающая миграция получает освободившийся slot;
7. stale lease от отсутствующей или terminal migration перехватывается;
8. lease, принадлежащий текущей migration, не блокирует повторный reconcile;
9. concurrent `TryAcquire` не выдаёт один и тот же slot двум migrations одновременно;
10. release идемпотентен;
11. VMOP для ожидающей inbound slot миграции остаётся в `Pending`, а не переходит в `Failed`.

## Нерешённые вопросы

1. Достаточно ли хранить holder по `VirtualMachineInstanceMigration`, или удобнее привязывать lease к VMI и текущему migration UID из `MigrationState`?
2. Делать ли `parallelInboundMigrationsPerNode` публичной настройкой сразу или оставить фиксированным `1`?
3. Нужно ли добавлять новый API reason в `VMOP`, или достаточно существующего `MigrationPending` с уточнённым message?
4. Где именно в KubeVirt ожидании `migrationConfiguration` лучше исключить waiting period из timeout-а: останавливать timer или игнорировать timeout result при наличии annotation?

## Рекомендация

Реализовать inbound migration limit через задержку выдачи `MigrationConfiguration` в `virtualization-controller` до получения Lease на target node.

Target node должен выбирать Kubernetes scheduler. `virtualization-controller` только читает результат scheduling-а из KubeVirt/VMI state и использует его для Lease-based limiter-а.

На первом этапе использовать фиксированный лимит `1`, waiting annotations и patch KubeVirt timeout-а ожидания migration parameters. В `VMOP` отображать ожидание как `Pending`, не переводя операцию в `Failed`.
