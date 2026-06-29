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

Реализовать inbound migration limit в `virtualization-controller` через задержку выдачи `MigrationConfiguration` до получения inbound slot на target node. Slot выдаёт **in-memory limiter внутри контроллера**, без отдельного Kubernetes-ресурса.

Общий flow:

1. `VirtualMachineInstanceMigration` создаётся как сейчас.
2. KubeVirt создаёт target pod.
3. Kubernetes scheduler назначает target pod на target node.
4. `livemigration-controller` определяет target node.
5. Перед patch-ем `KVVMI.status.migrationState.migrationConfiguration` controller пытается получить inbound slot через in-memory limiter.
6. Если slot получен, controller выставляет `MigrationConfiguration`, и миграция продолжается.
7. Если slot занят, controller не выставляет `MigrationConfiguration`, помечает VMI annotation-ом ожидания и requeue-ит reconcile.
8. KubeVirt patch в месте ожидания `migrationConfiguration` не считает timeout, пока VMI помечена как ожидающая inbound slot. Очередь на inbound slot не имеет таймаута: ожидающая миграция не должна превращаться в `Failed`.
9. Slot освобождается при завершении миграции, а после рестарта контроллера состояние восстанавливается из наблюдаемого состояния KubeVirt migration.

In-memory выбран осознанно: gate уже выполняется в единственном leader-инстансе `virtualization-controller`, поэтому достаточно процессного состояния под mutex-ом — это даёт сериализацию concurrent reconcile без нового кластерного ресурса, RBAC и cleanup-логики stale leases. Kubernetes `Lease` рассмотрен и отклонён (см. Альтернативы).

По умолчанию лимит:

```text
parallelInboundMigrationsPerNode = 1
```

Лимит конфигурируется аннотацией на ModuleConfig (см. Конфигурация), применяется при рестарте контроллера и может быть полностью отключён аннотацией. Механизм проектируется как slot-based limiter: один slot соответствует одной входящей миграции на target node, лимит `1` — частный случай с одним slot.

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
if !inboundLimiter.Enabled() {
    kvvmi.Status.MigrationState.MigrationConfiguration = conf
    return
}

targetNode := resolveTargetNode(kvvmi)
if targetNode == "" {
    return requeue
}

acquired := inboundLimiter.TryAcquire(kvvmi, targetNode)
if !acquired {
    markInboundSlotWaiting(kvvmi, targetNode)
    return requeue
}

clearInboundSlotWaiting(kvvmi)
kvvmi.Status.MigrationState.MigrationConfiguration = conf
```

Если slot занят, `MigrationConfiguration` не выставляется. Это удерживает миграцию в уже существующем KubeVirt flow ожидания параметров миграции.

## Annotation model

Для явного состояния ожидания используются annotations на VMI:

```yaml
virtualization.deckhouse.io/inbound-migration-slot: waiting
virtualization.deckhouse.io/inbound-migration-target-node: <node>
```

Правила:

- если inbound slot не получен, controller выставляет `inbound-migration-slot=waiting` и target node;
- пока annotation имеет значение `waiting`, `MigrationConfiguration` не выставляется;
- когда slot получен, controller удаляет waiting annotations и выставляет `MigrationConfiguration`;
- отдельное состояние `acquired` не требуется: наличие `MigrationConfiguration` и отсутствие `waiting` достаточно для продолжения миграции.

Эти annotations нужны не только для диагностики, но и для корректной работы timeout-а ожидания migration parameters в KubeVirt.

## Timeout ожидания migration parameters

В KubeVirt есть ожидание `migrationConfiguration`. Если параметры не появились за заданное время, миграция может быть завершена ошибкой.

Очередь на inbound slot должна быть **без таймаута**. Если оставить таймаут, вторая и последующие миграции на ту же target node будут штатно стоять в очереди, но падать по timeout — и мы получим множество VM с `Failed` VMOP вместо ожидания.

Поэтому для inbound limiter поведение KubeVirt нужно изменить:

```text
Если MigrationConfiguration == nil
и VMI имеет annotation virtualization.deckhouse.io/inbound-migration-slot=waiting,
то timeout ожидания migration parameters не тикает и не приводит к failed migration.
```

Если annotation `waiting` отсутствует, существующее поведение timeout-а сохраняется. Это важно, чтобы реальные проблемы с выдачей migration parameters не маскировались inbound limiter-ом.

## In-memory slot model

Limiter хранит занятые slots в памяти `virtualization-controller`.

Структура — реестр занятых slots по target node, защищённый mutex-ом:

```text
targetNode -> { ownerKey: slotIndex }
```

где `ownerKey` идентифицирует владельца slot (migration/VMI namespace/name/uid).

Правила:

- gate выполняется в единственном leader-инстансе контроллера, поэтому mutex даёт сериализацию между concurrent reconcile workers без кластерного ресурса;
- занятость slot — это запись в in-memory реестре; число slots на target node равно текущему лимиту;
- запись идемпотентна по `ownerKey`: повторный reconcile той же миграции не занимает второй slot, а возвращает уже выданный;
- release удаляет только запись текущего owner-а;
- состояние не персистентно: при рестарте/смене leader реестр пуст и восстанавливается из наблюдаемого состояния (см. Release и восстановление).

Привязка slot к target node ведётся по имени node из `MigrationState`. Holder-метаданные (namespace/name/uid миграции или VMI) хранятся в той же in-memory записи и используются для идемпотентности и восстановления.

## TryAcquire

`TryAcquire(kvvmi, targetNode)` должен работать так:

1. Под mutex-ом получить (или создать пустой) реестр slots для `targetNode`.
2. Если slot уже принадлежит текущей migration/VMI (`ownerKey` совпадает), вернуть `true` идемпотентно.
3. Если занятых slots меньше лимита, занять свободный slot текущим owner-ом и вернуть `true`.
4. Если все slots заняты другими active migrations, вернуть `false`.

Так как все операции выполняются под общим mutex-ом в одном процессе, гонок между reconcile workers нет: два worker-а не получат один и тот же последний slot.

## Release и восстановление

Slot должен освобождаться при завершении миграции:

- `VirtualMachineInstanceMigration` перешла в terminal phase;
- migration завершилась `Completed`/`Failed` на уровне отслеживаемого состояния;
- VMI больше не находится в live migration state;
- VMI/migration-владелец удалён.

`Release(kvvmi, targetNode)` должен быть идемпотентным:

1. Под mutex-ом найти запись текущего owner-а в реестре `targetNode`.
2. Если записи нет, завершиться успешно.
3. Если запись принадлежит текущему owner-у, удалить её.

Восстановление после рестарта/смены leader: in-memory реестр пуст и наполняется при reconcile. Контроллер пересматривает текущее состояние KubeVirt migrations и заново занимает slots для миграций, которые уже находятся в активной фазе (например, уже имеют `MigrationConfiguration` или активный `MigrationState`) на каждой target node. Миграции, прошедшие gate до рестарта, продолжают выполняться; новые ожидающие миграции встают в очередь после восстановления учёта.

Чтобы временно не превысить лимит сразу после рестарта (реестр ещё пуст), `TryAcquire` сверяет занятость не только с in-memory записями, но и с наблюдаемым числом active inbound migrations на target node. In-memory реестр служит для сериализации и идемпотентности, а наблюдаемое состояние KubeVirt — источником истины при восстановлении.

## Статусы и условия

Ожидающая inbound slot миграция не должна считаться failed. `k get vmop` должен оставаться понятным: операция в очереди отображается как `Pending` с явным сообщением про ожидание свободного inbound slot, а не как `Failed` и не «зависшей» без объяснения.

На уровне KubeVirt migration и VMI ожидание отражается через annotation:

```text
virtualization.deckhouse.io/inbound-migration-slot=waiting
virtualization.deckhouse.io/inbound-migration-target-node=<node>
```

На уровне `VirtualMachineOperation` используется существующий pending mapping:

```text
VMOP.status.phase: Pending
Completed condition:
  status: False
  reason: MigrationPending
  message: уточнённое сообщение про ожидание свободного inbound slot на target node.
```

Сообщение `MigrationPending` уточняется для случая ожидания inbound slot, чтобы по `k get vmop` и `describe` было видно причину ожидания. Добавление нового API reason возможно позже, но для первого этапа не обязательно.

## Конфигурация

Лимит конфигурируется аннотацией на ModuleConfig модуля virtualization. Значение пробрасывается в `virtualization-controller` через Helm-темплейты (hook читает аннотацию ModuleConfig и формирует internal values, темплейт деплоймента контроллера рендерит их в аргумент/переменную окружения). Значение читается контроллером на старте, поэтому **применяется при рестарте контроллера** — динамическая переконфигурация без рестарта не предполагается.

### Лимит

```yaml
virtualization.deckhouse.io/parallel-inbound-migrations-per-node: "1"
```

- по умолчанию `1`;
- значение `> 1` увеличивает число inbound slots на target node.

Так как upstream KubeVirt `MigrationConfiguration` не содержит такого поля, эта настройка Deckhouse-specific и применяется в логике `virtualization-controller`, а не как поле upstream `MigrationConfiguration`. Внутренний values path:

```text
virtualization.internal.virtConfig.parallelInboundMigrationsPerNode
```

### Отключение

Отдельная аннотация полностью отключает inbound limiter:

```yaml
virtualization.deckhouse.io/inbound-migration-limit: "disabled"
```

При `disabled` gate не применяется: `MigrationConfiguration` выставляется как сейчас, waiting annotations не используются, поведение полностью совпадает с текущим (до этого ADR). Это аварийный/диагностический выключатель на случай проблем с limiter-ом. Отключение также применяется при рестарте контроллера.

## Изменяемые компоненты

Основные места реализации:

- `images/virtualization-artifact/pkg/livemigration/inbound_limiter.go` — in-memory limiter (`TryAcquire`/`Release`/`Enabled`);
- `images/virtualization-artifact/pkg/controller/livemigration/internal/dynamic_settings_handler.go` — gate перед выставлением `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/livemigration/live_migration_reconciler.go` — release/восстановление на завершении миграции;
- `images/virtualization-artifact/pkg/livemigration/migration_configuration.go` — существующая генерация `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/vmop/migration/internal/handler/lifecycle.go` — отображение ожидания в статус `VMOP`;
- KubeVirt patch в месте ожидания `migrationConfiguration` — исключить период `inbound-migration-slot=waiting` из timeout-а ожидания parameters;
- `images/hooks/pkg/hooks/migration-config/hook.go` и `templates/kubevirt/_kubevirt_helpers.tpl` — чтение аннотаций ModuleConfig и проброс лимита/флага отключения в контроллер.

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

Суть: встроить limiter непосредственно в KubeVirt migration control loop.

Преимущества:

- близко к месту управления lifecycle KubeVirt migration;
- можно блокировать продвижение фаз напрямую.

Недостатки:

- больше patch surface в KubeVirt;
- сложнее поддерживать при обновлениях upstream;
- в проекте уже есть Deckhouse-specific gate ожидания `MigrationConfiguration`, который решает задачу меньшим изменением.

Решение отклонено в пользу gate через `MigrationConfiguration`.

### Альтернатива 3: slot-учёт через Kubernetes `Lease`

Суть: хранить занятые slots как `Lease` из `coordination.k8s.io/v1` в namespace `d8-virtualization` (по одному lease на slot target node), с `holderIdentity`, `leaseDurationSeconds`, optimistic concurrency для захвата и перехвата stale leases.

Преимущества:

- состояние переживает рестарт и смену leader контроллера;
- строгая защита от race condition между процессами через optimistic concurrency API.

Недостатки:

- требуется новый кластерный ресурс и RBAC на `leases`;
- нужна логика stale recovery, перехвата по `resourceVersion`, cleanup leases за пределами текущего лимита;
- gate и так выполняется в единственном leader-инстансе, поэтому межпроцессная синхронизация не требуется — in-memory mutex закрывает гонки между reconcile workers меньшими средствами;
- больше движущихся частей и операционной сложности ради сценария, который покрывается восстановлением из наблюдаемого состояния KubeVirt.

Решение отклонено в пользу in-memory limiter.

### Альтернатива 4: простой подсчёт активных миграций без сериализации

Суть: перед выдачей `MigrationConfiguration` list-ить все migrations и считать active incoming на target node, без in-memory реестра.

Недостатки:

- нет строгой сериализации при concurrent reconcile;
- возможны race conditions при одновременной выдаче slot;
- поведение зависит от cache freshness.

Используется как часть восстановления и как дополнительная сверка, но не как единственный механизм. In-memory реестр под mutex-ом добавляет сериализацию поверх подсчёта.

### Альтернатива 5: mutating webhook и init container gate

Суть: модифицировать target pod через webhook и удерживать его через init container до получения inbound slot.

Недостатки:

- сложнее операционно;
- требует вмешательства в pod lifecycle;
- уже есть более подходящая точка ожидания `MigrationConfiguration`;
- возможны побочные эффекты для KubeVirt target pod lifecycle.

Решение отклонено.

### Альтернатива 6: reactive abort/retry

Суть: разрешить KubeVirt начать миграцию, а при превышении inbound limit abort-ить или retry-ить лишние миграции.

Недостатки:

- на короткое время лимит может быть превышен;
- создаёт лишние abort/retry циклы;
- хуже пользовательский опыт;
- сложнее отличать штатную очередь от ошибки.

Решение отклонено.

## Последствия

### Положительные

- На target node будет не более настроенного числа активных входящих live migrations; по умолчанию — не более одной.
- Target node по-прежнему выбирает Kubernetes scheduler.
- Не нужно реализовывать собственную scheduler logic в `virtualization-controller`.
- Остальные миграции ждут в очереди без таймаута, а не падают в `Failed`.
- Ожидание использует уже существующий gate `MigrationConfiguration`.
- Ограничение работает независимо от источника миграции.
- In-memory limiter не требует нового кластерного ресурса, RBAC и cleanup stale leases.
- Лимит можно увеличить или полностью отключить аннотацией на ModuleConfig.
- Снижается риск перегрузки target node сетью, CPU, памятью и storage attach операциями.

### Отрицательные

- Состояние limiter-а не персистентно: после рестарта/смены leader его нужно восстанавливать из наблюдаемого состояния KubeVirt migrations.
- Требуется patch KubeVirt timeout-а ожидания `migrationConfiguration`.
- Появляется Deckhouse-specific поведение, которое нужно учитывать при обновлении KubeVirt.
- Изменение лимита/отключение требует рестарта контроллера.
- Возможна меньшая скорость массовой эвакуации, если много VM мигрируют на одну target node.
- Нужно аккуратно синхронизировать annotations, in-memory учёт и status patch VMI.

## План реализации

### Шаг 1. Добавить in-memory inbound limiter в `virtualization-controller`

Добавить компонент примерно такого вида:

```go
type InboundMigrationLimiter interface {
    Enabled() bool
    TryAcquire(kvvmi *virtv1.VirtualMachineInstance, targetNode string) bool
    Release(kvvmi *virtv1.VirtualMachineInstance, targetNode string)
}
```

Реализация хранит занятые slots в памяти под mutex-ом. Лимит и флаг отключения читаются на старте контроллера.

### Шаг 2. Интегрировать limiter в `DynamicSettingsHandler.Handle`

Логика:

1. если limiter отключён, выставить `MigrationConfiguration` как сейчас;
2. определить target node из `kvvmi.Status.MigrationState.TargetNode` или target pod `spec.nodeName`;
3. если target node неизвестна, requeue без выбора node;
4. вызвать `TryAcquire`;
5. если slot не получен, выставить waiting annotations и не выставлять `MigrationConfiguration`;
6. если slot получен, удалить waiting annotations и выставить `MigrationConfiguration`.

### Шаг 3. Изменить KubeVirt timeout ожидания parameters

В KubeVirt patch-е ожидания `migrationConfiguration` добавить правило:

```text
VMI with virtualization.deckhouse.io/inbound-migration-slot=waiting is waiting for inbound slot and must not fail by migration parameters timeout.
```

Очередь на inbound slot не имеет таймаута.

### Шаг 4. Добавить release и восстановление

Логика:

1. при terminal/completed/failed migration освободить slot owner-а;
2. при пустом реестре после рестарта восстанавливать учёт из наблюдаемого состояния active migrations;
3. сделать release идемпотентным.

### Шаг 5. Синхронизировать диагностику в VMOP

Если VMI имеет `inbound-migration-slot=waiting`, `vmop-migration-controller` должен отображать операцию как pending, а не failed, с понятным `k get vmop`:

- `VMOP.status.phase = Pending`;
- `Completed.reason = MigrationPending`;
- message содержит информацию про ожидание свободного inbound slot на target node.

### Шаг 6. Проброс конфигурации

- добавить чтение аннотаций ModuleConfig (`parallel-inbound-migrations-per-node`, `inbound-migration-limit`) в hook;
- пробросить значения в internal values и в аргумент/переменную окружения контроллера через темплейты;
- значения применяются при рестарте контроллера.

### Шаг 7. Тесты

Нужны unit/integration тесты:

1. одна миграция на target node получает slot и получает `MigrationConfiguration`;
2. при лимите `1` вторая миграция на ту же target node получает waiting annotations и не получает `MigrationConfiguration`;
3. KubeVirt timeout ожидания parameters не fail-ит VMI с `inbound-migration-slot=waiting`;
4. migration без waiting annotation сохраняет существующее timeout-поведение;
5. миграция на другую target node получает свой slot и продолжается;
6. после завершения первой миграции ожидающая миграция получает освободившийся slot;
7. повторный reconcile той же migration идемпотентен и не занимает второй slot;
8. concurrent `TryAcquire` не выдаёт один и тот же slot двум migrations одновременно;
9. release идемпотентен;
10. после рестарта учёт восстанавливается из наблюдаемого состояния и лимит не превышается;
11. при отключении аннотацией gate не применяется и поведение совпадает с текущим;
12. VMOP для ожидающей inbound slot миграции остаётся в `Pending`, а не переходит в `Failed`.

## Нерешённые вопросы

1. Достаточно ли вести owner по `VirtualMachineInstanceMigration`, или удобнее привязывать slot к VMI и текущему migration UID из `MigrationState`?
2. Как именно восстанавливать учёт после рестарта: считать active migrations с уже выставленным `MigrationConfiguration`, либо ориентироваться на фазу `MigrationState` target pod.
3. Нужно ли добавлять новый API reason в `VMOP`, или достаточно существующего `MigrationPending` с уточнённым message.
4. Где именно в KubeVirt ожидании `migrationConfiguration` лучше исключить waiting period из timeout-а: останавливать timer или игнорировать timeout result при наличии annotation.

## Рекомендация

Реализовать inbound migration limit через задержку выдачи `MigrationConfiguration` в `virtualization-controller` до получения inbound slot из in-memory limiter-а.

Target node должен выбирать Kubernetes scheduler. `virtualization-controller` только читает результат scheduling-а из KubeVirt/VMI state и использует его для slot-based limiter-а.

По умолчанию использовать лимит `1`, waiting annotations и patch KubeVirt timeout-а ожидания migration parameters так, чтобы очередь на inbound slot была без таймаута. В `VMOP` отображать ожидание как `Pending` с понятным сообщением, не переводя операцию в `Failed`. Лимит конфигурировать аннотацией на ModuleConfig с применением по рестарту контроллера и предусмотреть аннотацию для полного отключения ограничения.
