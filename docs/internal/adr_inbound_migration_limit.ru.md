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

Миграция может перейти к активной фазе только если на её target node нет другой активной входящей миграции.

Если target node уже занята другой active incoming migration, текущая миграция остаётся в ожидающем состоянии и повторно reconcile-ится позже.

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

    acquired, err := incomingLimiter.TryAcquire(ctx, migration, targetNode)
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

Чтобы гарантировать `<= 1`, limiter должен использовать атомарный механизм захвата slot.

Рекомендуемая реализация — Kubernetes `Lease` из `coordination.k8s.io/v1`.

### Lease model

Для каждой target node создаётся lease:

```text
namespace: d8-virtualization
name: incoming-migration-<safe-node-name>
holderIdentity: <migration-uid>
```

Правила:

- если lease отсутствует, миграция создаёт его со своим `UID`;
- если lease существует и `holderIdentity` равен `UID` текущей миграции, миграция продолжает выполнение;
- если lease существует и принадлежит другой non-final миграции, текущая миграция остаётся pending;
- если lease существует, но владелец уже terminal или отсутствует, lease можно перехватить;
- release удаляет lease или очищает `holderIdentity`, только если lease принадлежит текущей миграции.

### Детали реализации Lease

Lease должен быть отдельным служебным объектом, который представляет один inbound slot конкретной target node.

Рекомендуемый формат имени:

```text
incoming-migration-<node-name-hash>
```

Использовать только raw node name в имени нежелательно: имя node может быть длинным или содержать символы, которые потребуют нормализации. Поэтому безопаснее формировать имя из стабильного hash, а исходное имя node хранить в label или annotation.

Рекомендуемый объект:

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  namespace: d8-virtualization
  name: incoming-migration-<node-name-hash>
  labels:
    virtualization.deckhouse.io/component: inbound-migration-limiter
    virtualization.deckhouse.io/target-node-hash: <node-name-hash>
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

`TryAcquire(ctx, migration, targetNode)` должен работать так:

1. Построить lease name по `targetNode`.
2. Выполнить `Get` lease.
3. Если lease не найден:
   - создать lease с holder текущей migration;
   - если create завершился conflict/already exists, повторить `Get` и перейти к обычной проверке владельца.
4. Если lease найден и принадлежит текущей migration:
   - обновить `renewTime`;
   - вернуть `true`.
5. Если lease найден и принадлежит другой migration:
   - проверить, жива ли migration-владелец;
   - если владелец существует и не terminal, вернуть `false`;
   - если владелец отсутствует или terminal, попытаться перехватить lease через `Update` с текущим `resourceVersion`.
6. Если update завершился conflict, вернуть retryable error или повторить короткий цикл reread/update.

Псевдокод:

```go
func (l *LeaseIncomingMigrationLimiter) TryAcquire(ctx context.Context, mig *virtv1.VirtualMachineInstanceMigration, targetNode string) (bool, error) {
    lease, err := l.getLease(ctx, targetNode)
    if apierrors.IsNotFound(err) {
        return l.createLease(ctx, mig, targetNode)
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

1. Получить lease по target node.
2. Если lease отсутствует — успешно завершить.
3. Если lease принадлежит другой migration — ничего не делать.
4. Если lease принадлежит текущей migration — удалить lease.
5. Если delete получил `NotFound` — успешно завершить.

Удаление lease предпочтительнее очистки `holderIdentity`, потому что отсутствие lease проще обрабатывать в `TryAcquire`, а stale пустые lease не будут накапливаться.

### Renew

Так как lease используется не для leader election, а как атомарный slot, постоянный renew не обязателен. Достаточно обновлять `renewTime` при каждом reconcile migration, которая уже владеет lease.

`leaseDurationSeconds` нужен только как дополнительная диагностическая и safety-информация. Нельзя освобождать lease только по истечению времени, если migration-владелец всё ещё существует и не terminal: долгие live migrations допустимы.

### Требования к client/cache

Операции `Get/Create/Update/Delete` для Lease желательно выполнять через non-cached client или APIReader, если это доступно в месте интеграции. Это снижает риск решений на устаревшем cache.

Даже при cached read корректность должна обеспечиваться optimistic concurrency Kubernetes API:

- создать lease сможет только одна migration;
- перехват stale lease выполняется через `resourceVersion`;
- conflict приводит к повторному reconcile.

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
message: Target node already has an active incoming migration.
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

Преимущества:

- минимальные изменения публичного API;
- не требует новых ModuleConfig параметров;
- закрывает исходное требование.

### Возможное развитие

Позже можно сделать настройку конфигурируемой через ModuleConfig annotation и Helm values:

```yaml
virtualization.deckhouse.io/parallel-inbound-migrations-per-node: "1"
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

- На target node будет не более одной активной входящей live migration.
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
    TryAcquire(ctx context.Context, migration *virtv1.VirtualMachineInstanceMigration, targetNode string) (bool, error)
    Release(ctx context.Context, migration *virtv1.VirtualMachineInstanceMigration, targetNode string) error
}
```

Реализация должна использовать `coordination.k8s.io/v1 Lease`.

### Шаг 3. Интегрировать limiter в migration reconcile

Логика:

1. определить target node;
2. если миграция входит в active incoming phase — вызвать `TryAcquire`;
3. если slot занят — оставить migration pending и requeue;
4. если slot получен — продолжить стандартный flow;
5. на terminal phase вызвать `Release`.

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

1. одна миграция на target node получает lease и продолжается;
2. вторая миграция на ту же target node остаётся pending;
3. миграция на другую target node продолжается;
4. после завершения первой миграции вторая получает lease;
5. stale lease от отсутствующей migration перехватывается;
6. lease, принадлежащий текущей migration, не блокирует повторный reconcile;
7. concurrent `TryAcquire` не выдаёт slot двум migration одновременно.

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

Реализовать limiter в patched KubeVirt `virt-controller` через Kubernetes Lease.

На первом этапе использовать фиксированный лимит `1`, без изменения публичного API. В `VMOP` отображать ожидание как `Pending`, не переводя операцию в `Failed`.
