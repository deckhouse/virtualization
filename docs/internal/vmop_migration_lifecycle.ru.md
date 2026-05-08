# Жизненный цикл, причины и прогресс миграции VMOP

Этот документ описывает внутреннюю модель статусов, используемую при обработке миграции `VirtualMachineOperation`.
Документ предназначен для сопровождающих контроллеров и ревьюеров.

## Область применения

Контроллер миграции VMOP показывает состояние миграции через:

- `status.phase`
- `status.progress`
- `status.conditions[Completed].reason`
- `status.conditions[Completed].message`

Контроллер не копирует фазы миграции KubeVirt один к одному. Он преобразует фазы KubeVirt, состояние целевого pod, состояние миграции и метрики передачи в пользовательские причины VMOP и значения прогресса.

## Жизненный цикл выполнения

| Состояние миграции KubeVirt | Причина VMOP Completed | Фаза VMOP | Прогресс |
|---|---|---|---:|
| `MigrationPhaseUnset` | `MigrationPending` | `Pending` | `0%` |
| `MigrationPending` | `MigrationPending` | `Pending` | `0%` |
| `MigrationScheduling` | `TargetScheduling` | `InProgress` | `2%` |
| целевой pod не может быть запланирован | `TargetUnschedulable` | `InProgress` | `2%` |
| `MigrationScheduled` | `TargetPreparing` | `InProgress` | `3%` |
| `MigrationPreparingTarget` | `TargetPreparing` | `InProgress` | `3%` |
| у целевого pod есть ошибки подключения или монтирования дисков | `TargetDiskError` | `InProgress` | `3%` |
| `MigrationTargetReady` | `Syncing` | `InProgress` | динамический, `10..90%` |
| `MigrationWaitingForSync` | `Syncing` | `InProgress` | динамический, `10..90%` |
| `MigrationSynchronizing` | `Syncing` | `InProgress` | динамический, `10..90%` |
| `MigrationRunning` | `Syncing` | `InProgress` | динамический, `10..90%` |
| прогресс синхронизации останавливается при максимальном throttling | `NotConverging` | `InProgress` | динамический, `10..90%` |
| `MigrationState.Completed == true` | `SourceSuspended` | `InProgress` | `91%` |
| `TargetNodeDomainReadyTimestamp != nil` | `TargetResumed` | `InProgress` | `92%` |
| `MigrationSucceeded` | `Completed` | `Completed` | `100%` |

## Семантика причин

| Причина | Значение |
|---|---|
| `MigrationPending` | Объект миграции существует или скоро будет обработан, но планирование целевого pod еще не началось. |
| `TargetScheduling` | Планирование целевого pod началось. Операция уже активна, поэтому фаза VMOP — `InProgress`. |
| `TargetUnschedulable` | Целевой pod находится в состоянии pending и имеет Kubernetes-условие `PodScheduled=False, Unschedulable`. |
| `TargetPreparing` | Целевой pod был запланирован и подготавливается. |
| `TargetDiskError` | Целевой pod заблокирован из-за проблемы с диском, volume, CSI, подключением или монтированием. |
| `Syncing` | Источник и цель синхронизируют данные миграции. |
| `NotConverging` | Фаза синхронизации не показывает достаточного прогресса для сходимости. |
| `SourceSuspended` | Исходная VM была приостановлена в рамках финальной передачи миграции. |
| `TargetResumed` | Целевая VM возобновила работу на целевом узле. |
| `Completed` | Миграция успешно завершена. |
| `Aborted` | Миграция была прервана. |
| `Failed` | Миграция завершилась ошибкой без уточненной причины. |

## Модель прогресса

Фиксированные значения прогресса:

| Причина | Прогресс |
|---|---:|
| `MigrationPending` | `0%` |
| `DisksPreparing` | `1%` |
| `TargetScheduling` | `2%` |
| `TargetUnschedulable` | `2%` |
| `TargetPreparing` | `3%` |
| `TargetDiskError` | `3%` |
| `SourceSuspended` | `91%` |
| `TargetResumed` | `92%` |
| `Completed` | `100%` |

Динамические значения прогресса:

| Причина | Источник прогресса |
|---|---|
| `Syncing` | `migration/internal/progress.Progress.SyncProgress` |
| `NotConverging` | `migration/internal/progress.Progress.SyncProgress` |

Поведение при fallback:

| Причина | Поведение прогресса |
|---|---|
| `Aborted` | Сохранить текущее значение `vmop.status.progress`. |
| `Failed` | Сохранить текущее значение `vmop.status.progress`. |
| неизвестная причина | Сохранить текущее значение `vmop.status.progress`. |

## Детали прогресса синхронизации

Прогресс синхронизации намеренно консервативен и использует диапазон `10..90%`.

Стратегия синхронизации состоит из двух стадий:

| Стадия | Диапазон | Как выбирается |
|---|---:|---|
| bulk-стадия | `10..45%` | Стадия синхронизации по умолчанию, когда метрики итеративной миграции недоступны. |
| итеративная стадия | `45..90%` | Включается только при `transferStatus.iteration > 0`. |

Контроллер строит запись прогресса из:

- `migrationState.startTimestamp`
- `migrationState.mode`
- `migrationState.transferStatus.iteration`
- `migrationState.transferStatus.autoConvergeThrottle`
- `migrationState.transferStatus.dataTotalBytes`
- `migrationState.transferStatus.dataProcessedBytes`
- `migrationState.transferStatus.dataRemainingBytes`
- `migrationState.migrationConfiguration.allowAutoConverge`
- предыдущего значения `vmop.status.progress`

Если `transferStatus.iteration` отсутствует или равен нулю, миграция остается в bulk-стадии, а прогресс ограничивается `45%`, пока жизненный цикл не перейдет к `SourceSuspended`, `TargetResumed` или `Completed`.

Это означает, что многие быстрые миграции или миграции с неполными метриками могут визуально оставаться ниже `50%`, а затем перейти сразу к `91%`, `92%` или `100%`.

## Обнаружение NotConverging

`NotConverging` может появиться двумя способами.

### Обнаружение во время выполнения

Во время `Syncing` контроллер вызывает `progressStrategy.IsNotConverging(record)`.

Стратегия возвращает `true` только при выполнении всех условий:

1. Для VMOP есть сохраненное состояние прогресса.
2. Миграция вошла в итеративную стадию.
3. Миграция находится на максимальном throttling:
   - если auto-converge отключен, это считается максимальным throttling;
   - если auto-converge включен, требуется `transferStatus.autoConvergeThrottle >= 99`.
4. Минимальное наблюдаемое количество оставшихся данных не улучшалось как минимум `10s`.

Когда это происходит, VMOP остается в фазе `InProgress`, но `Completed.reason` становится `NotConverging`, а сообщение становится:

```text
Migration is not converging: data remaining is not decreasing at maximum throttle
```

### Обнаружение терминальной ошибки

Когда KubeVirt сообщает `MigrationFailed`, `getFailedReason` также сопоставляет ошибку с `NotConverging`, если `migrationState.failureReason` содержит:

- `converg`
- `progress`

Если терминальная ошибка в остальном общая, но предыдущий `Completed.reason` у VMOP был `NotConverging`, контроллер сохраняет `NotConverging` как финальную причину.

## Обработка неуспешной миграции

Когда KubeVirt сообщает `MigrationFailed`, контроллер устанавливает:

- `vmop.status.phase = Failed`
- `conditions[Completed].status = False`
- `conditions[Completed].reason = <классифицированная причина>`
- `status.progress = <прогресс, зависящий от причины>`

Порядок классификации причины ошибки:

| Приоритет | Условие | Финальная причина | Прогресс |
|---:|---|---|---:|
| 1 | `migrationState.abortRequested == true` или `abortStatus == MigrationAbortSucceeded` | `Aborted` | сохранить текущий |
| 2 | `migrationState.failureReason` содержит `converg` или `progress` | `NotConverging` | динамический, `10..90%` |
| 3 | причина или сообщение failed-условия содержит `schedul` или `unschedul` | `TargetUnschedulable` | `2%` |
| 4 | причина или сообщение failed-условия содержит `csi`, `attach`, `volume` или `disk` | `TargetDiskError` | `3%` |
| 5 | нет специфичного совпадения | `Failed` | сохранить текущий |

Базовое сообщение ошибки по причине:

| Причина | Базовое сообщение |
|---|---|
| `Aborted` | `Migration aborted` |
| `NotConverging` | `Migration did not converge` |
| `TargetUnschedulable` | `Migration failed: target pod is unschedulable` |
| `TargetDiskError` | `Migration failed: target disk attach error` |
| `Failed` | `Migration failed` |

Контроллер добавляет к базовому сообщению дополнительные детали из:

1. `migrationState.failureReason`, если он есть;
2. иначе — из сообщения условия KubeVirt `VirtualMachineInstanceMigrationFailed`, если оно есть.

## Диагностика целевого pod

Пока миграция выполняется, диагностика целевого pod может переопределить причину, основанную на фазе.

### Целевой pod не может быть запланирован

Если целевой pod находится в pending и имеет:

```text
PodScheduled=False, Reason=Unschedulable
```

то VMOP использует:

- причина: `TargetUnschedulable`
- прогресс: `2%`
- сообщение: `Target pod "<namespace>/<name>" is unschedulable`

### Ошибка подключения или монтирования диска

Если целевой pod находится в состоянии создания контейнера, а warning events содержат:

- `FailedAttachVolume`
- `FailedMount`

то VMOP использует:

- причина: `TargetDiskError`
- прогресс: `3%`
- сообщение: `Target pod has disk attach error: <event reason>: <event message>`

## Проекция условия VM

Контроллер VM проецирует причины миграции VMOP в условие VM `Migrating`.

Важные сообщения:

| Причина VMOP | Сообщение VM |
|---|---|
| `MigrationPending` | `Migration is awaiting start.` |
| `TargetScheduling` | `Migration is in progress: target pod is being scheduled.` |
| `MigrationPrepareTarget`, `TargetPreparing`, `DisksPreparing` | `Migration is in progress: target pod is being scheduled and prepared.` |
| `MigrationTargetReady`, `Syncing`, `SourceSuspended`, `TargetResumed` | `Migration is in progress: source and target are being synchronized.` |

## Практические примеры

| Сценарий | Фаза VMOP | Причина | Прогресс |
|---|---|---|---:|
| Объект миграции существует, но KubeVirt еще не начал планирование | `Pending` | `MigrationPending` | `0%` |
| Планирование целевого pod началось | `InProgress` | `TargetScheduling` | `2%` |
| Целевой pod не может быть запланирован | `InProgress` или `Failed` | `TargetUnschedulable` | `2%` |
| У целевого pod есть ошибки подключения volume | `InProgress` или `Failed` | `TargetDiskError` | `3%` |
| Синхронизация без метрик итераций | `InProgress` | `Syncing` | `10..45%` |
| Синхронизация с метриками итераций | `InProgress` | `Syncing` | `45..90%` |
| Синхронизация останавливается при максимальном throttling | `InProgress` или `Failed` | `NotConverging` | динамический |
| Миграция прервана | `Failed` | `Aborted` | сохранить текущий |
| Неизвестная ошибка | `Failed` | `Failed` | сохранить текущий |
| Источник приостановлен во время финальной передачи | `InProgress` | `SourceSuspended` | `91%` |
| Цель возобновлена | `InProgress` | `TargetResumed` | `92%` |
| Миграция успешно завершена | `Completed` | `Completed` | `100%` |
