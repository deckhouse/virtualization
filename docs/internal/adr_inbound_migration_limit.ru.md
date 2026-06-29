# ADR: ограничение входящих live migrations на target node

## Описание

Документ предлагает ограничивать количество одновременных входящих live migrations на одну target node в модуле virtualization. Ограничение реализуется в `virtualization-controller` через задержку выдачи `MigrationConfiguration`: миграция допускается к активной фазе только после получения inbound slot у in-memory limiter-а внутри контроллера. По умолчанию на target node допускается не более одной входящей миграции; остальные штатно ждут свободный slot без таймаута, а не падают в `Failed`. Target node по-прежнему выбирает Kubernetes scheduler, а контроллер только читает результат scheduling-а. Решение реализовано.

### Контекст

В модуле virtualization live migration выполняется через KubeVirt `VirtualMachineInstanceMigration`. Пользовательские и автоматические сценарии миграции в Deckhouse проходят через несколько уровней:

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

Из-за этого платформа умеет ограничивать количество исходящих миграций с source node, но не умеет ограничивать количество входящих миграций на target node.

В проекте уже есть механизм ожидания динамических параметров миграции, который используется как точка подключения:

- `virtualization-controller` вычисляет параметры через `images/virtualization-artifact/pkg/livemigration/migration_configuration.go`;
- `livemigration-controller` патчит `KVVMI.status.migrationState.migrationConfiguration`;
- KubeVirt/virt-launcher ждёт `migrationConfiguration` перед продолжением миграции.

Основная точка подключения:

```text
images/virtualization-artifact/pkg/controller/livemigration/internal/dynamic_settings_handler.go
DynamicSettingsHandler.Handle(ctx, kvvmi)
```

До этого ADR handler безусловно выставлял:

```go
kvvmi.Status.MigrationState.MigrationConfiguration = conf
```

Эта точка используется как gate для inbound migration limit: `MigrationConfiguration` не выставляется, пока target node не получила inbound slot.

## Мотивация / Боль

На практике несколько VM могут одновременно мигрировать на одну и ту же target node, даже если для source nodes ограничение уже работает. Это создаёт риск перегрузки target node сетью, CPU, памятью и storage attach операциями.

Требование: контролировать, что входящих миграций на target node не более одной (в общем случае — не более настроенного числа). Остальные миграции должны штатно ожидать свободный inbound slot, а не завершаться ошибкой.

Ограничение нельзя надёжно реализовать до создания `VirtualMachineInstanceMigration`, потому что target node становится известна только после создания target pod и его назначения scheduler-ом. Контроллер не должен заранее выбирать target node — иначе ему пришлось бы повторять часть логики Kubernetes scheduler и KubeVirt placement (nodeSelector, placement самой VM, taints/tolerations, affinities, resources, devices, storage constraints, динамику scheduling state), и всё равно не было бы гарантии, что KubeVirt и scheduler выберут именно проверенную node.

Ограничение должно применяться не только к миграциям из пользовательского `VMOP`, но и к другим источникам.

### Пользовательские истории

#### История 1: массовая эвакуация ноды

Администратор уводит нагрузку с ноды (drain/eviction). Десятки VM начинают мигрировать. Без inbound-ограничения несколько из них одновременно приходят на одну свободную ноду и перегружают её. С ограничением миграции на одну target node выполняются по очереди, по одной за раз, остальные ждут.

#### История 2: автоматическая системная миграция

Миграция создана не через `VMOP`, а workload-updater-ом или напрямую через KubeVirt API. Ограничение должно действовать и для неё — поэтому gate ставится не на уровне `VMOP`, а на общем для всех источников этапе ожидания `MigrationConfiguration`.

## Область

Документ затрагивает выдачу `MigrationConfiguration` в `virtualization-controller`, отображение ожидания в статусе `VMOP` и проброс конфигурации лимита через ModuleConfig.

### Цели

- На target node одновременно не более настроенного числа активных входящих live migrations (по умолчанию — одна).
- Остальные миграции ждут в очереди без таймаута, а не падают в `Failed`.
- Ограничение работает независимо от источника миграции (VMOP, eviction, drain, workload updater, прямой KubeVirt API).
- Target node по-прежнему выбирает Kubernetes scheduler; собственная scheduler logic в контроллере не реализуется.
- Ожидание видно в `k get vmop` как понятный `Pending`, а не как `Failed` или «зависание».
- Лимит можно увеличить или полностью отключить аннотацией на ModuleConfig.

### Не цели

- Не выбирать target node заранее в контроллере и не повторять логику scheduler/placement.
- Не вводить новый кластерный ресурс, RBAC и cleanup stale leases.
- Не патчить KubeVirt и не вмешиваться в target pod lifecycle.
- Не поддерживать динамическую переконфигурацию лимита без рестарта контроллера.

## Детальное описание решения

Inbound migration limit реализуется через задержку выдачи `MigrationConfiguration` до получения inbound slot на target node. Slot выдаёт **in-memory limiter внутри контроллера**, без отдельного Kubernetes-ресурса.

Общий flow:

1. `VirtualMachineInstanceMigration` создаётся как сейчас.
2. KubeVirt создаёт target pod.
3. Kubernetes scheduler назначает target pod на target node.
4. `livemigration-controller` определяет target node.
5. Перед patch-ем `KVVMI.status.migrationState.migrationConfiguration` controller пытается получить inbound slot через in-memory limiter.
6. Если slot получен, controller выставляет `MigrationConfiguration`, и миграция продолжается.
7. Если slot занят, controller не выставляет `MigrationConfiguration`, помечает VMI annotation-ом ожидания и requeue-ит reconcile.
8. Очередь на inbound slot не имеет таймаута: ожидающая миграция не превращается в `Failed`. Это обеспечивается существующим поведением KubeVirt, дополнительный patch не требуется (см. ниже).
9. Slot освобождается при завершении миграции, а после рестарта контроллера учёт восстанавливается сканированием VMI по annotation `inbound-migration-slot=acquired`.

In-memory выбран осознанно: gate выполняется в единственном leader-инстансе `virtualization-controller`, поэтому достаточно процессного состояния под mutex-ом — это даёт сериализацию concurrent reconcile без нового кластерного ресурса, RBAC и cleanup-логики stale leases. Kubernetes `Lease` рассмотрен и отклонён (см. Рассмотренные альтернативы).

По умолчанию лимит:

```text
parallelInboundMigrationsPerNode = 1
```

Механизм проектируется как slot-based limiter: один slot соответствует одной входящей миграции на target node, лимит `1` — частный случай с одним slot.

**Target node.** Target node выбирает Kubernetes scheduler. `livemigration-controller` определяет target node из доступного состояния KubeVirt:

1. `kvvmi.Status.MigrationState.TargetNode`, если поле заполнено;
2. `kvvmi.Status.MigrationState.TargetPod` → `pod.spec.nodeName`, если target pod уже создан и назначен scheduler-ом.

Если target node ещё неизвестна, inbound limiter не блокирует миграцию и не пытается выбрать node самостоятельно — controller ждёт следующего reconcile.

**Gate через MigrationConfiguration.** `MigrationConfiguration` становится точкой допуска миграции к активной фазе. Логика в `DynamicSettingsHandler.Handle`:

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

Если slot занят, `MigrationConfiguration` не выставляется, и миграция остаётся в существующем KubeVirt flow ожидания параметров миграции.

**Annotation model.** Состояние slot хранится в annotations на VMI:

```yaml
virtualization.deckhouse.io/inbound-migration-slot: waiting | acquired
virtualization.deckhouse.io/inbound-migration-target-node: <node>
```

Правила:

- если inbound slot не получен, controller выставляет `inbound-migration-slot=waiting` и target node;
- пока annotation имеет значение `waiting`, `MigrationConfiguration` не выставляется;
- когда slot получен, controller выставляет `inbound-migration-slot=acquired`, target node и `MigrationConfiguration`;
- значение `acquired` — точка восстановления: по нему контроллер при старте понимает, что VMI уже держит slot на target node.

Эти annotations нужны для диагностики и для восстановления in-memory учёта после рестарта контроллера.

**In-memory slot model.** Limiter хранит занятые slots в памяти, реестр по target node под mutex-ом:

```text
targetNode -> { ownerKey }
```

где `ownerKey` идентифицирует владельца slot по `namespace/name/migrationUID` VMI.

Правила:

- gate выполняется в единственном leader-инстансе контроллера, поэтому mutex даёт сериализацию между concurrent reconcile workers без кластерного ресурса;
- занятость slot — это запись в in-memory реестре; число slots на target node равно текущему лимиту;
- запись идемпотентна по `ownerKey`: повторный reconcile той же миграции не занимает второй slot, а возвращает уже выданный;
- release удаляет только запись текущего owner-а;
- состояние не персистентно в памяти, но восстанавливается из annotations на VMI.

`TryAcquire(kvvmi, targetNode)`:

1. Под mutex-ом получить (или создать пустой) реестр slots для `targetNode`.
2. Если slot уже принадлежит текущей migration/VMI (`ownerKey` совпадает), вернуть `true` идемпотентно.
3. Если занятых slots меньше лимита, занять свободный slot текущим owner-ом и вернуть `true`.
4. Если все slots заняты другими active migrations, вернуть `false`.

Так как все операции выполняются под общим mutex-ом в одном процессе, два worker-а не получат один и тот же последний slot.

**Release и восстановление.** Slot освобождается при завершении миграции (terminal phase `VirtualMachineInstanceMigration`, `Completed`/`Failed` в отслеживаемом состоянии, VMI вышла из live migration state, VMI/migration-владелец удалён). `Release(kvvmi, targetNode)` идемпотентен: под mutex-ом находит запись текущего owner-а в реестре `targetNode` и удаляет её; если записи нет — завершается успешно.

Восстановление после рестарта/смены leader выполняется из annotations на VMI. При старте, до начала обработки очереди, контроллер:

1. list-ит VMI и отбирает те, у которых есть `inbound-migration-slot` и `inbound-migration-target-node`;
2. для каждой VMI со значением `acquired` занимает slot на соответствующей target node, восстанавливая запись реестра;
3. VMI со значением `waiting` не занимают slot — они заново пройдут `TryAcquire` при ближайшем reconcile;
4. перед использованием annotation проверяется актуальность: если VMI уже не в live migration state или migration terminal, annotation считается устаревшей и slot не занимается.

Так число занятых slots на каждой target node восстанавливается до того, как новые миграции начнут получать `MigrationConfiguration`, поэтому лимит не превышается сразу после рестарта.

**Timeout ожидания migration parameters.** Очередь на inbound slot должна быть без таймаута. Анализ кода собираемого форка `deckhouse/3p-kubevirt` (`v1.6.2-v12n`) показал, что **отдельный patch KubeVirt не требуется** — ожидание `migrationConfiguration` уже происходит без таймаута:

1. Patch «External migration configuration» в `pkg/virt-handler/migration-source.go` при `MigrationState.MigrationConfiguration == nil` просто прерывает reconcile (`return nil`) и ждёт следующего апдейта VMI — таймера здесь нет.
2. Единственные таймауты миграции (`handlePendingPodTimeout` в `pkg/virt-controller/watch/migration/migration.go`: unschedulable 5 мин, catch-all 15 мин) срабатывают только пока target pod в фазе `Pending`.
3. `virtualization-controller` выдаёт slot и `MigrationConfiguration` только когда `MigrationState != nil`, а это поле создаётся virt-controller-ом на handoff (Scheduled → PreparingTarget), то есть когда target pod уже `Running`. К моменту ожидания slot окно pending-таймаута уже закрыто, и withholding `MigrationConfiguration` не удерживает pod в `Pending`.
4. В фазах `PreparingTarget`/`TargetReady` миграция фейлится только если target pod упал (`!targetPodExists || PodIsDown`), без таймаута по времени.

Единственный остаточный таймаут — catch-all pending timeout target pod-а — срабатывает только если pod реально не может зашедулиться (нет ресурсов на ноде), не связан с inbound limiter-ом и является штатным желаемым поведением KubeVirt.

### Имплементация

Основные места реализации:

- `images/virtualization-artifact/pkg/livemigration/inbound_limiter.go` — in-memory limiter (`Enabled`/`TryAcquire`/`Release`) и восстановление из annotations;
- `images/virtualization-artifact/pkg/controller/livemigration/internal/dynamic_settings_handler.go` — gate перед выставлением `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/livemigration/live_migration_controller.go` — построение limiter-а и восстановление учёта на старте;
- `images/virtualization-artifact/pkg/livemigration/migration_configuration.go` — существующая генерация `MigrationConfiguration`;
- `images/virtualization-artifact/pkg/controller/vmop/migration/internal/handler/lifecycle.go` — отображение ожидания в статус `VMOP`;
- `images/hooks/pkg/hooks/migration-config/hook.go`, `openapi/values.yaml` и `templates/virtualization-controller/_helpers.tpl` — чтение аннотаций ModuleConfig и проброс лимита/флага отключения в контроллер через env.

Порядок работ:

1. **In-memory inbound limiter.** Компонент с интерфейсом:

   ```go
   type InboundMigrationLimiter interface {
       Enabled() bool
       TryAcquire(kvvmi *virtv1.VirtualMachineInstance, targetNode string) bool
       Release(kvvmi *virtv1.VirtualMachineInstance, targetNode string)
   }
   ```

   Реализация хранит занятые slots в памяти под mutex-ом. Лимит и флаг отключения читаются на старте контроллера.

2. **Интеграция в `DynamicSettingsHandler.Handle`.** Если limiter отключён — выставить `MigrationConfiguration` как раньше; иначе определить target node, при неизвестной node — requeue, вызвать `TryAcquire`, при неудаче — waiting annotations без `MigrationConfiguration`, при успехе — `acquired` annotation и `MigrationConfiguration`.

3. **KubeVirt timeout — patch не требуется** (см. раздел про timeout выше).

4. **Release и восстановление.** При terminal/completed/failed migration освободить slot owner-а и очистить annotations VMI; при старте контроллера восстанавливать учёт по annotation `acquired`, отбрасывая устаревшие; release идемпотентен.

5. **Проброс конфигурации.** Чтение аннотаций ModuleConfig (`parallel-inbound-migrations-per-node`, `inbound-migration-limit`) в hook, проброс в internal values и в env контроллера через темплейты; значения применяются при рестарте контроллера.

### Пользовательский опыт

**Диагностика ожидания.** Ожидающая inbound slot миграция не считается failed. На уровне KubeVirt migration и VMI ожидание отражается через annotation:

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

Сообщение `MigrationPending` уточняется для случая ожидания inbound slot, чтобы по `k get vmop` и `describe` была видна причина ожидания.

**Конфигурация лимита.** Лимит конфигурируется аннотацией на ModuleConfig модуля virtualization:

```yaml
virtualization.deckhouse.io/parallel-inbound-migrations-per-node: "1"
```

- по умолчанию `1`;
- значение `> 1` увеличивает число inbound slots на target node.

Так как upstream KubeVirt `MigrationConfiguration` не содержит такого поля, настройка Deckhouse-specific и применяется в логике `virtualization-controller`. Внутренние values path:

```text
virtualization.internal.virtConfig.parallelInboundMigrationsPerNode
virtualization.internal.virtConfig.inboundMigrationLimit
```

Значения пробрасываются в `virtualization-controller` через env-переменные деплоймента (`PARALLEL_INBOUND_MIGRATIONS_PER_NODE`, `INBOUND_MIGRATION_LIMIT`) в `templates/virtualization-controller/_helpers.tpl` и читаются на старте контроллера, поэтому **применяются при рестарте контроллера**.

**Отключение.** Отдельная аннотация полностью отключает inbound limiter:

```yaml
virtualization.deckhouse.io/inbound-migration-limit: "disabled"
```

При `disabled` gate не применяется: `MigrationConfiguration` выставляется как раньше, waiting annotations не используются, поведение полностью совпадает с поведением до этого ADR. Это аварийный/диагностический выключатель. Отключение также применяется при рестарте контроллера.

### Тестирование

Unit/integration тесты:

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

## Минусы внедрения решения

- Состояние limiter-а не персистентно: после рестарта/смены leader его нужно восстанавливать из наблюдаемого состояния KubeVirt migrations.
- При обновлении KubeVirt нужно проверять, что ожидание `migrationConfiguration` по-прежнему происходит без таймаута; если upstream введёт таймаут, его придётся исключать для VMI с `inbound-migration-slot=waiting`.
- Изменение лимита/отключение требует рестарта контроллера.
- Возможна меньшая скорость массовой эвакуации, если много VM мигрируют на одну target node.
- Нужно аккуратно синхронизировать annotations, in-memory учёт и status patch VMI.

## Рассмотренные альтернативы

### Альтернатива 1: предварительно выбирать target node в `vmop-migration-controller`

Суть: до создания `VirtualMachineInstanceMigration` выбрать target node и не создавать migration, если node занята.

Недостатки: target node должен выбирать Kubernetes scheduler; controller должен повторить scheduler logic; нет гарантии, что KubeVirt выберет проверенную node; не покрывает миграции не через `VMOP`; возможны гонки между несколькими VMOP.

Решение отклонено.

### Альтернатива 2: limiter внутри patched KubeVirt `virt-controller`

Суть: встроить limiter непосредственно в KubeVirt migration control loop.

Преимущества: близко к месту управления lifecycle KubeVirt migration; можно блокировать продвижение фаз напрямую.

Недостатки: больше patch surface в KubeVirt; сложнее поддерживать при обновлениях upstream; в проекте уже есть Deckhouse-specific gate ожидания `MigrationConfiguration`, который решает задачу меньшим изменением.

Решение отклонено в пользу gate через `MigrationConfiguration`.

### Альтернатива 3: slot-учёт через Kubernetes `Lease`

Суть: хранить занятые slots как `Lease` из `coordination.k8s.io/v1` в namespace `d8-virtualization` (по одному lease на slot target node), с `holderIdentity`, `leaseDurationSeconds`, optimistic concurrency.

Преимущества: состояние переживает рестарт и смену leader контроллера; строгая защита от race condition между процессами.

Недостатки: требуется новый кластерный ресурс и RBAC на `leases`; нужна логика stale recovery, перехвата по `resourceVersion`, cleanup leases; gate и так выполняется в единственном leader-инстансе, поэтому межпроцессная синхронизация не требуется — in-memory mutex закрывает гонки между reconcile workers меньшими средствами; больше движущихся частей ради сценария, который покрывается восстановлением из наблюдаемого состояния KubeVirt.

Решение отклонено в пользу in-memory limiter.

### Альтернатива 4: простой подсчёт активных миграций без сериализации

Суть: перед выдачей `MigrationConfiguration` list-ить все migrations и считать active incoming на target node, без in-memory реестра.

Недостатки: нет строгой сериализации при concurrent reconcile; возможны race conditions при одновременной выдаче slot; поведение зависит от cache freshness.

Может использоваться как дополнительная сверка, но не как основной механизм.

### Альтернатива 5: mutating webhook и init container gate

Суть: модифицировать target pod через webhook и удерживать его через init container до получения inbound slot.

Недостатки: сложнее операционно; требует вмешательства в pod lifecycle; уже есть более подходящая точка ожидания `MigrationConfiguration`; возможны побочные эффекты для KubeVirt target pod lifecycle.

Решение отклонено.

### Альтернатива 6: reactive abort/retry

Суть: разрешить KubeVirt начать миграцию, а при превышении inbound limit abort-ить или retry-ить лишние миграции.

Недостатки: на короткое время лимит может быть превышен; лишние abort/retry циклы; хуже пользовательский опыт; сложнее отличать штатную очередь от ошибки.

Решение отклонено.

## Вопросы на будущее и дальнейшие планы

- Нужно ли добавлять новый API reason в `VMOP`, или достаточно существующего `MigrationPending` с уточнённым message. Для первого этапа выбран уточнённый `MigrationPending`.
- При обновлении версии KubeVirt проверять, что ожидание `migrationConfiguration` остаётся без таймаута; иначе добавить исключение по annotation `inbound-migration-slot=waiting`.
- Возможна более строгая дополнительная сверка in-memory учёта со списком active migrations (Альтернатива 4) как защита от рассинхронизации.

Закрытые в процессе реализации вопросы:

- Owner slot ведётся по `namespace/name/migrationUID` VMI (а не по `VirtualMachineInstanceMigration`).
- Patch KubeVirt для timeout-а ожидания `migrationConfiguration` не требуется на текущем форке.

## Ответственные контактные лица

- Команда модуля Deckhouse Virtualization — реализация и сопровождение `virtualization-controller` и `livemigration-controller`.
- Сопровождающие форка `deckhouse/3p-kubevirt` — на случай, если будущая версия KubeVirt изменит поведение ожидания `migrationConfiguration`.
