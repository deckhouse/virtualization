# ADR: ограничение входящих live migrations на target node

## Статус

Предложено.

## Контекст

В модуле virtualization live migration выполняется через KubeVirt `VirtualMachineInstanceMigration`.
Пользовательский и автоматический сценарии миграции в Deckhouse проходят через несколько уровней:

1. `VirtualMachineOperation` (`VMOP`) создаётся пользователем, контроллером эвакуации, workload-updater или другим компонентом.
2. `vmop-migration-controller` создаёт KubeVirt-ресурс `VirtualMachineInstanceMigration`.
3. KubeVirt `virt-controller` планирует target pod и управляет жизненным циклом live migration.
4. Контроллеры virtualization синхронизируют статус KubeVirt migration обратно в `VMOP` и `VirtualMachine`.

Сейчас ограничение параллелизма задаётся через KubeVirt `MigrationConfiguration`:

```yaml
parallelMigrationsPerCluster: <N>
parallelOutboundMigrationsPerNode: <N>
```

В проекте это значение прокидывается из Helm/templates и hooks:

- `templates/kubevirt/_kubevirt_helpers.tpl`
- `images/hooks/pkg/hooks/migration-config/hook.go`
- `images/virtualization-artifact/pkg/livemigration/migration_configuration.go`

При этом KubeVirt API не содержит симметричной настройки вида:

```yaml
parallelInboundMigrationsPerNode: <N>
```

Из-за этого платформа умеет ограничивать количество исходящих миграций с source node, но не умеет ограничивать количество входящих миграций на target node. На практике несколько VM могут одновременно мигрировать на одну и ту же target node, даже если для source nodes ограничение уже работает.

Требование: контролировать, что входящих миграций на target node не более одной. Остальные миграции должны ожидать в `Pending` или другом подходящем состоянии, а не завершаться ошибкой.

## Проблема

Ограничение нельзя надёжно реализовать только в `vmop-migration-controller`, потому что target node обычно становится известна после создания `VirtualMachineInstanceMigration`, когда KubeVirt уже начал планировать target pod.

Если пытаться решить задачу до создания KubeVirt migration, придётся повторять часть логики Kubernetes scheduler и KubeVirt placement:

- учитывать `nodeSelector` из `VMOP.spec.migrate.nodeSelector`;
- учитывать placement самого `VirtualMachine`;
- учитывать taints/tolerations, affinities, resources, devices, storage constraints;
- учитывать динамические изменения node и pod scheduling state.

Такой подход будет неполным и не даст строгой гарантии, что KubeVirt выберет именно ту node, которую предварительно проверил controller.

Также ограничение должно применяться не только к миграциям, созданным через пользовательский `VMOP`, но и к другим источникам миграций:

- eviction;
- node drain;
- workload updater;
- автоматические системные миграции;
- миграции, созданные напрямую через KubeVirt API.

Поэтому правильная точка контроля — KubeVirt migration control loop, где уже известен target node и где принимается решение о продвижении миграции по фазам.

## Решение

Добавить в KubeVirt `virt-controller` внутренний limiter входящих миграций на target node.

На первом этапе лимит фиксированный:

```text
maxIncomingMigrationsPerNode = 1
```

При этом механизм должен проектироваться не как single-lock, а как slot-based limiter: один `Lease` соответствует одному inbound slot на target node. Лимит `1` является частным случаем с одним slot. Если в будущем потребуется разрешить, например, `5` одновременных входящих миграций на target node, controller будет использовать пять lease-slots для этой node.

Миграция может перейти к активной фазе только если на её target node есть свободный inbound slot или slot уже принадлежит этой миграции.

Если все inbound slots target node заняты другими active incoming migrations, текущая миграция остаётся в ожидающем состоянии и повторно reconcile-ится позже.

## Определения

### Target node

Target node — node, на которую KubeVirt планирует перенести VMI.

Источник target node зависит от текущей фазы миграции:

- `VirtualMachineInstanceMigration.Status.MigrationState.TargetNode`, если уже заполнено;
- target pod `spec.nodeName`, если target pod уже создан и назначен scheduler-ом;
- для более ранних фаз target node может быть ещё неизвестна, и limiter не должен блокировать миграцию до появления target node.

### Active incoming migration

Active incoming migration — миграция, которая:

1. не находится в terminal phase;
2. имеет target node;
3. уже потребляет или скоро начнёт потреблять ресурсы target node как live migration target.

Рекомендуемый набор фаз, которые считать активными:

```text
MigrationScheduled
MigrationPreparingTarget
MigrationTargetReady
MigrationWaitingForSync
MigrationSynchronizing
MigrationRunning
```

Фазы, которые не считаются активными:

```text
MigrationPhaseUnset
MigrationPending
MigrationSucceeded
MigrationFailed
```

`MigrationScheduling` можно не считать активной, если target pod ещё не назначен на node. Если target pod уже имеет `spec.nodeName`, миграция может участвовать в inbound limiting даже на фазе `MigrationScheduling`.

## Алгоритм

### 1. До появления target node

Если target node неизвестна, миграция продолжает обычный KubeVirt flow.

Limiter не должен пытаться выбирать target node самостоятельно.

### 2. После назначения target node

Перед переходом миграции в активную фазу controller проверяет inbound capacity target node.

Псевдокод:

```go
func reconcileMigration(migration *VirtualMachineInstanceMigration) error {
    targetNode := resolveTargetNode(migration)
    if targetNode == "" {
        return continueDefaultMigrationFlow(migration)
    }

    if !isEnteringActiveIncomingPhase(migration) {
        return continueDefaultMigrationFlow(migration)
    }

    acquired, err := incomingLimiter.TryAcquire(ctx, migration, targetNode, parallelInboundMigrationsPerNode)
    if err != nil {
        return err
    }

    if !acquired {
        setMigrationPending(migration, "TargetNodeIncomingMigrationLimitExceeded")
        return requeueAfter(defaultMigrationRequeueDelay)
    }

    return continueDefaultMigrationFlow(migration)
}
```

### 3. Завершение миграции

При переходе миграции в terminal phase limiter освобождает занятый slot:

```go
if migration.IsFinal() {
    incomingLimiter.Release(ctx, migration, targetNode)
}
```

Также release должен быть идемпотентным и безопасным при повторном reconcile.

## Синхронизация и защита от race condition

Простой подсчёт активных миграций по списку `VirtualMachineInstanceMigration` недостаточен для строгой гарантии. При нескольких workers возможна гонка:

1. две миграции одновременно проверяют target node;
2. обе видят, что активных входящих миграций нет;
3. обе продолжают выполнение.

Чтобы гарантировать соблюдение лимита, limiter должен использовать атомарный механизм захвата slot.

Рекомендуемая реализация — Kubernetes `Lease` из `coordination.k8s.io/v1`.

### Lease model

Один `Lease` представляет один inbound slot target node.

При лимите `1` для target node доступен один slot:

```text
namespace: d8-virtualization
name: incoming-migration-<node-name-hash>-0
holderIdentity: <migration-namespace>/<migration-name>/<migration-uid>
```

При лимите `5` для той же target node доступны пять независимых slots:

```text
incoming-migration-<node-name-hash>-0
incoming-migration-<node-name-hash>-1
incoming-migration-<node-name-hash>-2
incoming-migration-<node-name-hash>-3
incoming-migration-<node-name-hash>-4
```

Правила:

- если один из slot leases отсутствует, миграция может создать его со своим holder;
- если один из slot leases уже принадлежит текущей миграции, миграция продолжает выполнение и обновляет `renewTime`;
- если slot lease принадлежит другой non-final миграции, этот slot считается занятым;
- если slot lease существует, но владелец уже terminal или отсутствует, slot можно перехватить;
- если все slots заняты другими active migrations, текущая миграция остаётся pending;
- release удаляет только тот slot lease, который принадлежит текущей миграции.

### Детали реализации Lease

Lease должен быть отдельным служебным объектом, который представляет один inbound slot конкретной target node.

Рекомендуемый формат имени:

```text
incoming-migration-<node-name-hash>-<slot-index>
```

`slot-index` — число от `0` до `parallelInboundMigrationsPerNode - 1`.

Использовать только raw node name в имени нежелательно: имя node может быть длинным или содержать символы, которые потребуют нормализации. Поэтому безопаснее формировать имя из стабильного hash, а исходное имя node хранить в label или annotation.

Рекомендуемый объект:

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  namespace: d8-virtualization
  name: incoming-migration-<node-name-hash>-<slot-index>
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

`holderIdentity` должен содержать не только UID, но и namespace/name. Это упрощает проверку владельца без list-а всех migrations во всех namespaces.

OwnerReference на `VirtualMachineInstanceMigration` добавлять не нужно, потому что migration namespaced, а lease хранится в namespace control plane. Cross-namespace owner reference для namespaced объектов некорректен. Очистка должна выполняться явно через `Release` и через stale lease recovery.

### TryAcquire

`TryAcquire(ctx, migration, targetNode, limit)` должен работать так:

1. Построить список lease names по `targetNode` и текущему лимиту: `0..parallelInboundMigrationsPerNode-1`.
2. Сначала проверить все slots и найти lease, который уже принадлежит текущей migration.
3. Если такой lease найден:
   - обновить `renewTime`;
   - вернуть `true`.
4. Если текущая migration ещё не владеет slot-ом, пройти по всем slots и попытаться занять первый доступный:
   - если lease не найден — создать lease с holder текущей migration;
   - если create завершился conflict/already exists — перейти к следующему reread/retry;
   - если lease принадлежит другой migration — проверить владельца;
   - если владелец существует и не terminal — считать slot занятым и перейти к следующему;
   - если владелец отсутствует или terminal — попытаться перехватить slot через `Update` с текущим `resourceVersion`.
5. Если один из slots успешно создан или перехвачен — вернуть `true`.
6. Если все slots заняты активными владельцами — вернуть `false`.
7. Если update завершился conflict, повторить короткий цикл reread/update или вернуть retryable error.

Псевдокод:

```go
func (l *LeaseIncomingMigrationLimiter) TryAcquire(ctx context.Context, mig *virtv1.VirtualMachineInstanceMigration, targetNode string, limit int) (bool, error) {
    slots := l.slotNames(targetNode, limit)

    for _, slot := range slots {
        lease, err := l.getLease(ctx, slot)
        if apierrors.IsNotFound(err) {
            continue
        }
        if err != nil {
            return false, err
        }
        if isHeldBy(lease, mig) {
            return true, l.renewLease(ctx, lease, mig)
        }
    }

    for _, slot := range slots {
        acquired, err := l.tryAcquireSlot(ctx, mig, targetNode, slot)
        if err != nil {
            if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
                continue
            }
            return false, err
        }
        if acquired {
            return true, nil
        }
    }

    return false, nil
}
```

`tryAcquireSlot` внутри выполняет create, проверку владельца и steal stale slot для одного конкретного lease name.

```go
func (l *LeaseIncomingMigrationLimiter) tryAcquireSlot(ctx context.Context, mig *virtv1.VirtualMachineInstanceMigration, targetNode string, slot string) (bool, error) {
    lease, err := l.getLease(ctx, slot)
    if apierrors.IsNotFound(err) {
        return l.createLease(ctx, mig, targetNode, slot)
    }
    if err != nil {
        return false, err
    }
    if isHeldBy(lease, mig) {
        return true, l.renewLease(ctx, lease, mig)
    }

    alive, err := l.holderMigrationIsActive(ctx, lease)
    if err != nil {
        return false, err
    }
    if alive {
        return false, nil
    }

    return l.stealLease(ctx, lease, mig, targetNode)
}
```

### Проверка владельца

Проверка владельца lease должна использовать annotations:

```text
virtualization.deckhouse.io/migration-namespace
virtualization.deckhouse.io/migration-name
virtualization.deckhouse.io/migration-uid
```

Алгоритм:

1. Если annotations неполные — считать lease stale.
2. Сделать `Get` `VirtualMachineInstanceMigration` по namespace/name из annotations.
3. Если объект не найден — lease stale.
4. Если UID объекта отличается от UID в annotation — lease stale.
5. Если migration находится в terminal phase — lease stale.
6. Иначе lease занят активной migration.

Terminal phases:

```text
MigrationSucceeded
MigrationFailed
```

### Release

`Release(ctx, migration, targetNode)` должен быть идемпотентным:

1. Построить список lease names по target node и текущему лимиту.
2. Найти slot lease, принадлежащий текущей migration.
3. Если такой lease отсутствует — успешно завершить.
4. Если lease принадлежит текущей migration — удалить lease.
5. Если delete получил `NotFound` — успешно завершить.

Если лимит был уменьшен после того, как migration заняла slot с индексом за пределами нового лимита, `Release` всё равно должен уметь найти и удалить её lease. Для этого release может дополнительно list-ить leases по labels `component=inbound-migration-limiter` и `target-node-hash=<hash>`, а затем фильтровать holder текущей migration.

Удаление lease предпочтительнее очистки `holderIdentity`, потому что отсутствие lease проще обрабатывать в `TryAcquire`, а stale пустые lease не будут накапливаться.

### Renew

Так как lease используется не для leader election, а как атомарный slot, постоянный renew не обязателен. Достаточно обновлять `renewTime` при каждом reconcile migration, которая уже владеет lease.

`leaseDurationSeconds` нужен только как дополнительная диагностическая и safety-информация. Нельзя освобождать lease только по истечению времени, если migration-владелец всё ещё существует и не terminal: долгие live migrations допустимы.

### Требования к client/cache

Операции `Get/Create/Update/Delete` для Lease желательно выполнять через non-cached client или APIReader, если это доступно в месте интеграции. Это снижает риск решений на устаревшем cache.

Даже при cached read корректность должна обеспечиваться optimistic concurrency Kubernetes API:

- конкретный slot lease сможет создать только одна migration;
- разные migrations могут одновременно занять разные slot leases в пределах лимита;
- перехват stale lease выполняется через `resourceVersion`;
- conflict приводит к проверке следующего slot или повторному reconcile.

### RBAC

`virt-controller` должен получить права на leases в namespace `d8-virtualization`:

```text
apiGroups: ["coordination.k8s.io"]
resources: ["leases"]
verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

`list/watch` нужны только если реализация использует informer/cache или периодический cleanup. Для минимальной реализации достаточно `get/create/update/delete`, но в controller-runtime окружении часто проще выдать полный набор read/write verbs для leases.

### Обработка stale lease

Lease может остаться после аварийного завершения controller-а или удаления migration resource.

При обнаружении занятого lease controller должен проверить владельца:

1. найти `VirtualMachineInstanceMigration` по namespace/name и сверить UID владельца;
2. если владелец отсутствует или terminal, считать lease stale;
3. перехватить lease через optimistic update с `resourceVersion`.

Дополнительно можно использовать `renewTime` и `leaseDurationSeconds`, но основной критерий освобождения — состояние migration owner.

## Статусы и условия

Ожидающая из-за inbound limit миграция не должна считаться failed.

Рекомендуемая модель статуса KubeVirt migration:

```text
phase: Pending
condition/reason: TargetNodeIncomingMigrationLimitExceeded
message: Target node has no free inbound migration slots.
```

На уровне `VirtualMachineOperation` можно использовать существующий pending mapping:

```text
VMOP.status.phase: Pending
Completed condition:
  status: False
  reason: MigrationPending
  message: The VirtualMachineOperation for migrating the virtual machine has been queued. Waiting for the queue to be processed and for this operation to be executed.
```

Для лучшей диагностики можно добавить новый reason в API virtualization:

```text
TargetNodeIncomingMigrationLimitExceeded
```

Но это потребует изменения API, CRD и документации. Для первого этапа достаточно сохранить `ReasonMigrationPending`, но заменить message на более точный, если KubeVirt condition содержит причину inbound limit.

## Конфигурация

### Первый этап

Лимит фиксированный:

```text
parallelInboundMigrationsPerNode = 1
```

Даже при фиксированном значении реализация должна использовать slot-based модель, чтобы изменение лимита до `5` или другого значения не требовало переделки алгоритма.

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

Но так как upstream KubeVirt `MigrationConfiguration` не содержит такого поля, эта настройка будет Deckhouse-specific и должна применяться только в patched `virt-controller`.

## Альтернативы

### Альтернатива 1: реализовать ограничение в `vmop-migration-controller`

Суть: перед созданием `VirtualMachineInstanceMigration` проверить target node и не создавать migration, если node занята.

Недостатки:

- target node чаще всего ещё неизвестна;
- controller должен повторить scheduler logic;
- нет гарантии, что KubeVirt выберет проверенную node;
- не покрывает миграции, созданные не через `VMOP`;
- возможны гонки между несколькими VMOP.

Решение отклонено.

### Альтернатива 2: простой подсчёт активных миграций без Lease

Суть: перед продолжением миграции list-ить все migrations и считать active incoming на target node.

Преимущества:

- проще реализации;
- не требует дополнительных ресурсов.

Недостатки:

- нет строгой гарантии при concurrent reconcile;
- возможны race conditions;
- поведение зависит от cache freshness.

Можно использовать как дополнительную проверку, но не как основной механизм гарантии.

Решение отклонено как основной вариант.

## Последствия

### Положительные

- На target node будет не более настроенного числа активных входящих live migrations; на первом этапе — не более одной.
- Остальные миграции будут ждать, а не падать.
- Ограничение будет работать независимо от источника миграции.
- Снижается риск перегрузки target node сетью, CPU, памятью и storage attach операциями.
- Поведение становится симметричнее текущему outbound limit.

### Отрицательные

- Требуется patch KubeVirt `virt-controller`.
- Появляется Deckhouse-specific поведение, которое нужно учитывать при обновлении KubeVirt.
- Появляется новый служебный ресурс `Lease` и логика очистки stale leases.
- Возможна меньшая скорость массовой эвакуации, если много VM мигрируют на одну target node.

## План реализации

### Шаг 1. Найти точку интеграции в KubeVirt

В patched `virt-controller` найти control loop, который продвигает `VirtualMachineInstanceMigration` по фазам и создаёт/контролирует target pod.

Нужно вставить limiter после того, как target node известна, но до начала активной live migration синхронизации.

### Шаг 2. Добавить incoming limiter

Добавить компонент примерно такого вида:

```go
type IncomingMigrationLimiter interface {
    TryAcquire(ctx context.Context, migration *virtv1.VirtualMachineInstanceMigration, targetNode string, limit int) (bool, error)
    Release(ctx context.Context, migration *virtv1.VirtualMachineInstanceMigration, targetNode string, limit int) error
}
```

Реализация должна использовать `coordination.k8s.io/v1 Lease`. Один Lease соответствует одному inbound slot; количество slots равно `limit`.

### Шаг 3. Интегрировать limiter в migration reconcile

Логика:

1. определить target node;
2. определить текущий inbound limit;
3. если миграция входит в active incoming phase — вызвать `TryAcquire`;
4. если все slots заняты — оставить migration pending и requeue;
5. если slot получен — продолжить стандартный flow;
6. на terminal phase вызвать `Release`.

### Шаг 4. Синхронизировать диагностику в virtualization-controller

Если KubeVirt migration получила reason `TargetNodeIncomingMigrationLimitExceeded`, `vmop-migration-controller` должен отображать это как pending состояние.

Минимальный вариант:

- `VMOP.status.phase = Pending`;
- `Completed.reason = MigrationPending`;
- message содержит информацию про занятый target node.

Расширенный вариант:

- добавить новый `vmopcondition.ReasonCompleted`;
- обновить CRD и документацию.

### Шаг 5. Тесты

Нужны unit/integration тесты для patched KubeVirt logic:

1. одна миграция на target node получает slot lease и продолжается;
2. при лимите `1` вторая миграция на ту же target node остаётся pending;
3. при лимите `5` пять миграций на одну target node получают разные slot leases;
4. при лимите `5` шестая миграция на ту же target node остаётся pending;
5. миграция на другую target node продолжается;
6. после завершения первой миграции ожидающая миграция получает освободившийся slot;
7. stale lease от отсутствующей migration перехватывается;
8. lease, принадлежащий текущей migration, не блокирует повторный reconcile;
9. concurrent `TryAcquire` не выдаёт один и тот же slot двум migration одновременно;
10. уменьшение лимита не мешает `Release` удалить slot lease, уже занятый текущей migration.

Для virtualization-controller нужны тесты mapping-а статуса:

1. KubeVirt migration pending из-за inbound limit отображается в `VMOP.status.phase = Pending`;
2. migration не переводится в failed;
3. message понятен пользователю.

## Нерешённые вопросы

1. Нужно ли считать `MigrationPreparingTarget` активной входящей миграцией или блокировать только начиная с `MigrationTargetReady`?
2. Делать ли `parallelInboundMigrationsPerNode` публичной настройкой сразу или оставить фиксированным `1`?
3. Нужно ли добавлять новый API reason в `VMOP`, или достаточно существующего `MigrationPending` с уточнённым message?
4. Где хранить lease: в namespace KubeVirt (`d8-virtualization`) или рядом с migration namespace?

## Рекомендация

Реализовать slot-based limiter в patched KubeVirt `virt-controller` через Kubernetes Lease: один Lease соответствует одному inbound slot target node.

На первом этапе использовать фиксированный лимит `1`, без изменения публичного API. При будущем переходе на лимит `5` или другое значение достаточно изменить количество доступных slots. В `VMOP` отображать ожидание как `Pending`, не переводя операцию в `Failed`.
