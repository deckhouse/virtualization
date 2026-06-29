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
8. Очередь на inbound slot не имеет таймаута: ожидающая миграция не должна превращаться в `Failed`. Это обеспечивается существующим поведением KubeVirt, дополнительный patch не требуется (см. Timeout ожидания migration parameters).
9. Slot освобождается при завершении миграции, а после рестарта контроллера учёт восстанавливается сканированием VMI по annotation `inbound-migration-slot=acquired`.

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

markInboundSlotAcquired(kvvmi, targetNode)
kvvmi.Status.MigrationState.MigrationConfiguration = conf
```

Если slot занят, `MigrationConfiguration` не выставляется. Это удерживает миграцию в уже существующем KubeVirt flow ожидания параметров миграции.

## Annotation model

Состояние slot хранится в annotations на VMI:

```yaml
virtualization.deckhouse.io/inbound-migration-slot: waiting | acquired
virtualization.deckhouse.io/inbound-migration-target-node: <node>
```

Правила:

- если inbound slot не получен, controller выставляет `inbound-migration-slot=waiting` и target node;
- пока annotation имеет значение `waiting`, `MigrationConfiguration` не выставляется;
- когда slot получен, controller выставляет `inbound-migration-slot=acquired`, target node и `MigrationConfiguration`;
- значение `acquired` — точка восстановления: по нему контроллер при старте понимает, что VMI уже держит slot на target node (см. In-memory slot model и Release и восстановление).

Эти annotations нужны для диагностики, для корректной работы timeout-а ожидания migration parameters в KubeVirt и для восстановления in-memory учёта после рестарта контроллера.

## Timeout ожидания migration parameters

Очередь на inbound slot должна быть **без таймаута**: вторая и последующие миграции на ту же target node должны штатно стоять в очереди, а не падать по timeout в `Failed`.

Анализ кода собираемого форка `deckhouse/3p-kubevirt` (`v1.6.2-v12n`) показал, что **отдельный patch KubeVirt не требуется**: ожидание `migrationConfiguration` уже происходит без таймаута.

Поток:

1. Patch «External migration configuration» в `pkg/virt-handler/migration-source.go` при `MigrationState.MigrationConfiguration == nil` просто прерывает reconcile (`return nil`) и ждёт следующего апдейта VMI — таймера здесь нет.
2. Единственные таймауты миграции (`handlePendingPodTimeout` в `pkg/virt-controller/watch/migration/migration.go`: unschedulable 5 мин, catch-all 15 мин) срабатывают только пока target pod в фазе `Pending`.
3. `virtualization-controller` выдаёт slot и `MigrationConfiguration` только когда `MigrationState != nil`, а это поле создаётся virt-controller-ом на handoff (Scheduled → PreparingTarget), то есть когда target pod уже `Running`. К моменту ожидания slot окно pending-таймаута уже закрыто, и withholding `MigrationConfiguration` не удерживает pod в `Pending`.
4. В фазах `PreparingTarget`/`TargetReady` миграция фейлится только если target pod упал (`!targetPodExists || PodIsDown`), без таймаута по времени.

Поэтому очередь на inbound slot и так получается без таймаута. Единственный остаточный таймаут — catch-all pending timeout target pod-а — срабатывает только если pod реально не может зашедулиться (нет ресурсов на ноде), не связан с inbound limiter-ом и является штатным желаемым поведением KubeVirt.

Если в будущем версия KubeVirt введёт таймаут на ожидании `migrationConfiguration`, его нужно будет исключить для VMI с annotation `virtualization.deckhouse.io/inbound-migration-slot=waiting`. На текущем форке это не требуется.

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
- состояние не персистентно в памяти, но восстанавливается из annotations на VMI: при старте контроллер сканирует VMI с `inbound-migration-slot=acquired` и переинициализирует реестр (см. Release и восстановление).

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

Восстановление после рестарта/смены leader выполняется из annotations на VMI. При старте, до начала обработки очереди, контроллер:

1. list-ит VMI и отбирает те, у которых есть `inbound-migration-slot` и `inbound-migration-target-node`;
2. для каждой VMI со значением `acquired` занимает slot на соответствующей target node, восстанавливая запись реестра с owner-ом этой VMI;
3. VMI со значением `waiting` не занимают slot — они заново пройдут `TryAcquire` при ближайшем reconcile;
4. перед использованием annotation проверяется актуальность: если VMI уже не в live migration state или migration terminal, annotation считается устаревшей, slot не занимается, а annotation очищается.

Так число занятых slots на каждой target node восстанавливается до того, как новые миграции начнут получать `MigrationConfiguration`, поэтому лимит не превышается сразу после рестарта. Миграции, прошедшие gate до рестарта (`acquired`), продолжают выполняться; ожидающие (`waiting`) встают в очередь после восстановления учёта.

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
virtualization.internal.virtConfig.inboundMigrationLimit
```

Значения пробрасываются в `virtualization-controller` через env-переменные деплоймента (`PARALLEL_INBOUND_MIGRATIONS_PER_NODE`, `INBOUND_MIGRATION_LIMIT`) в `templates/virtualization-controller/_helpers.tpl` и читаются на старте контроллера.

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
- `images/hooks/pkg/hooks/migration-config/hook.go`, `openapi/values.yaml` и `templates/virtualization-controller/_helpers.tpl` — чтение аннотаций ModuleConfig и проброс лимита/флага отключения в контроллер через env.

KubeVirt patch для timeout-а ожидания `migrationConfiguration` **не требуется** на текущем форке (см. Timeout ожидания migration parameters).

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

Может использоваться как дополнительная сверка, но не как основной механизм: учёт ведёт in-memory реестр под mutex-ом, а восстановление после рестарта идёт по annotations VMI.

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
- При обновлении KubeVirt нужно проверять, что ожидание `migrationConfiguration` по-прежнему происходит без таймаута; если upstream введёт таймаут, его придётся исключать для VMI с `inbound-migration-slot=waiting`.
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

### Шаг 3. KubeVirt timeout ожидания parameters — patch не требуется

Анализ собираемого форка `deckhouse/3p-kubevirt` (`v1.6.2-v12n`) показал, что ожидание `migrationConfiguration` уже происходит без таймаута, а pending-таймауты target pod-а не пересекаются с фазой ожидания slot (см. Timeout ожидания migration parameters). Очередь на inbound slot получается без таймаута без дополнительного patch-а.

### Шаг 4. Добавить release и восстановление

Логика:

1. при terminal/completed/failed migration освободить slot owner-а и очистить annotations VMI;
2. при старте контроллера сканировать VMI и восстанавливать учёт по annotation `inbound-migration-slot=acquired`, отбрасывая устаревшие annotations;
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
3. миграция на другую target node получает свой slot и продолжается;
4. после завершения первой миграции ожидающая миграция получает освободившийся slot;
5. повторный reconcile той же migration идемпотентен и не занимает второй slot;
6. concurrent `TryAcquire` не выдаёт один и тот же slot двум migrations одновременно;
7. release идемпотентен;
8. после рестарта учёт восстанавливается сканированием VMI по annotation `acquired` и лимит не превышается; устаревшие annotations отбрасываются;
9. при отключении аннотацией gate не применяется и поведение совпадает с текущим;
10. VMOP для ожидающей inbound slot миграции остаётся в `Pending`, а не переходит в `Failed`.

## Нерешённые вопросы

1. ~~Достаточно ли вести owner по `VirtualMachineInstanceMigration`, или удобнее привязывать slot к VMI и текущему migration UID из `MigrationState`?~~ Решено: owner ведётся по `namespace/name/migrationUID` VMI.
2. Нужно ли добавлять новый API reason в `VMOP`, или достаточно существующего `MigrationPending` с уточнённым message.
3. ~~Где именно в KubeVirt ожидании `migrationConfiguration` лучше исключить waiting period из timeout-а.~~ Снято: на текущем форке таймаута на ожидании `migrationConfiguration` нет, patch не требуется (см. Timeout ожидания migration parameters).

## Рекомендация

Реализовать inbound migration limit через задержку выдачи `MigrationConfiguration` в `virtualization-controller` до получения inbound slot из in-memory limiter-а.

Target node должен выбирать Kubernetes scheduler. `virtualization-controller` только читает результат scheduling-а из KubeVirt/VMI state и использует его для slot-based limiter-а.

По умолчанию использовать лимит `1` и waiting annotations. Очередь на inbound slot получается без таймаута без дополнительного patch-а KubeVirt: существующий gate `migrationConfiguration` ждёт без таймера, а pending-таймауты target pod-а не пересекаются с фазой ожидания slot. В `VMOP` отображать ожидание как `Pending` с понятным сообщением, не переводя операцию в `Failed`. Лимит конфигурировать аннотацией на ModuleConfig с применением по рестарту контроллера и предусмотреть аннотацию для полного отключения ограничения.
