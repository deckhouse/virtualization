# VMOP migration lifecycle, reasons, and progress

This document describes the internal status model used by `VirtualMachineOperation` migration handling.
It is intended for controller maintainers and reviewers.

## Scope

The VMOP migration controller exposes migration state through:

- `status.phase`
- `status.progress`
- `status.conditions[Completed].reason`
- `status.conditions[Completed].message`

The controller does not mirror KubeVirt migration phases one-to-one. It converts KubeVirt phases, target pod state, migration state, and transfer metrics into user-facing VMOP reasons and progress values.

## In-progress lifecycle

| KubeVirt migration state | VMOP Completed reason | VMOP phase | Progress |
|---|---|---|---:|
| `MigrationPhaseUnset` | `MigrationPending` | `Pending` | `0%` |
| `MigrationPending` | `MigrationPending` | `Pending` | `0%` |
| `MigrationScheduling` | `TargetScheduling` | `InProgress` | `2%` |
| target pod is unschedulable | `TargetUnschedulable` | `InProgress` | `2%` |
| `MigrationScheduled` | `TargetPreparing` | `InProgress` | `3%` |
| `MigrationPreparingTarget` | `TargetPreparing` | `InProgress` | `3%` |
| target pod has disk attach or mount errors | `TargetDiskError` | `InProgress` | `3%` |
| `MigrationTargetReady` | `Syncing` | `InProgress` | dynamic, `10..90%` |
| `MigrationWaitingForSync` | `Syncing` | `InProgress` | dynamic, `10..90%` |
| `MigrationSynchronizing` | `Syncing` | `InProgress` | dynamic, `10..90%` |
| `MigrationRunning` | `Syncing` | `InProgress` | dynamic, `10..90%` |
| sync progress stalls at maximum throttle | `NotConverging` | `InProgress` | dynamic, `10..90%` |
| `MigrationState.Completed == true` | `SourceSuspended` | `InProgress` | `91%` |
| `TargetNodeDomainReadyTimestamp != nil` | `TargetResumed` | `InProgress` | `92%` |
| `MigrationSucceeded` | `Completed` | `Completed` | `100%` |

## Reason semantics

| Reason | Meaning |
|---|---|
| `MigrationPending` | Migration object exists or is about to be processed, but target scheduling has not started yet. |
| `TargetScheduling` | Target pod scheduling has started. The operation is already active, so VMOP phase is `InProgress`. |
| `TargetUnschedulable` | The target pod is pending and has the Kubernetes `PodScheduled=False, Unschedulable` condition. |
| `TargetPreparing` | The target pod has been scheduled and is being prepared. |
| `TargetDiskError` | The target pod is stuck on a disk, volume, CSI, attach, or mount problem. |
| `Syncing` | Source and target are synchronizing migration data. |
| `NotConverging` | The sync phase is not making enough progress to converge. |
| `SourceSuspended` | Source VM has been suspended as part of the final migration handoff. |
| `TargetResumed` | Target VM has resumed on the target node. |
| `Completed` | Migration completed successfully. |
| `Aborted` | Migration was aborted. |
| `Failed` | Migration failed for an unspecified reason. |

## Progress model

Fixed progress values:

| Reason | Progress |
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

Dynamic progress values:

| Reason | Progress source |
|---|---|
| `Syncing` | `migration/internal/progress.Progress.SyncProgress` |
| `NotConverging` | `migration/internal/progress.Progress.SyncProgress` |

Fallback behavior:

| Reason | Progress behavior |
|---|---|
| `Aborted` | Keep current `vmop.status.progress`. |
| `Failed` | Keep current `vmop.status.progress`. |
| unknown reason | Keep current `vmop.status.progress`. |

## Sync progress details

Sync progress is intentionally conservative and uses the `10..90%` range.

The sync strategy has two stages:

| Stage | Range | How it is selected |
|---|---:|---|
| bulk stage | `10..45%` | Default sync stage when no iterative migration metrics are available. |
| iterative stage | `45..90%` | Enabled only when `transferStatus.iteration > 0`. |

The controller builds a progress record from:

- `migrationState.startTimestamp`
- `migrationState.mode`
- `migrationState.transferStatus.iteration`
- `migrationState.transferStatus.autoConvergeThrottle`
- `migrationState.transferStatus.dataTotalBytes`
- `migrationState.transferStatus.dataProcessedBytes`
- `migrationState.transferStatus.dataRemainingBytes`
- `migrationState.migrationConfiguration.allowAutoConverge`
- previous `vmop.status.progress`

If `transferStatus.iteration` is absent or zero, the migration remains in the bulk stage and progress is capped at `45%` until the lifecycle advances to `SourceSuspended`, `TargetResumed`, or `Completed`.

This means that many fast or metric-poor migrations may visually stay below `50%` and then jump to `91%`, `92%`, or `100%`.

## NotConverging detection

`NotConverging` can appear in two ways.

### In-progress detection

During `Syncing`, the controller calls `progressStrategy.IsNotConverging(record)`.

The strategy returns `true` only when all conditions are met:

1. There is stored progress state for the VMOP.
2. The migration has entered the iterative stage.
3. Migration is at maximum throttle:
   - if auto-converge is disabled, it is treated as maximum throttle;
   - if auto-converge is enabled, `transferStatus.autoConvergeThrottle >= 99` is required.
4. The minimum observed remaining data has not improved for at least `10s`.

When this happens, the VMOP stays `InProgress`, but `Completed.reason` becomes `NotConverging` and the message becomes:

```text
Migration is not converging: data remaining is not decreasing at maximum throttle
```

### Terminal failed detection

When KubeVirt reports `MigrationFailed`, `getFailedReason` also maps the failure to `NotConverging` if `migrationState.failureReason` contains:

- `converg`
- `progress`

If the terminal failure is otherwise generic but the previous VMOP `Completed.reason` was `NotConverging`, the controller keeps `NotConverging` as the final reason.

## Failed migration handling

When KubeVirt reports `MigrationFailed`, the controller sets:

- `vmop.status.phase = Failed`
- `conditions[Completed].status = False`
- `conditions[Completed].reason = <classified reason>`
- `status.progress = <reason-dependent progress>`

Failure reason classification order:

| Priority | Condition | Final reason | Progress |
|---:|---|---|---:|
| 1 | `migrationState.abortRequested == true` or `abortStatus == MigrationAbortSucceeded` | `Aborted` | keep current |
| 2 | `migrationState.failureReason` contains `converg` or `progress` | `NotConverging` | dynamic, `10..90%` |
| 3 | failed condition reason or message contains `schedul` or `unschedul` | `TargetUnschedulable` | `2%` |
| 4 | failed condition reason or message contains `csi`, `attach`, `volume`, or `disk` | `TargetDiskError` | `3%` |
| 5 | no specific match | `Failed` | keep current |

Failure message base by reason:

| Reason | Message base |
|---|---|
| `Aborted` | `Migration aborted` |
| `NotConverging` | `Migration did not converge` |
| `TargetUnschedulable` | `Migration failed: target pod is unschedulable` |
| `TargetDiskError` | `Migration failed: target disk attach error` |
| `Failed` | `Migration failed` |

The controller appends additional details to the base message from:

1. `migrationState.failureReason`, if present;
2. otherwise, the KubeVirt `VirtualMachineInstanceMigrationFailed` condition message, if present.

## Target pod diagnostics

While migration is in progress, target pod diagnostics can override the phase-based reason.

### Unschedulable target pod

If the target pod is pending and has:

```text
PodScheduled=False, Reason=Unschedulable
```

then VMOP uses:

- reason: `TargetUnschedulable`
- progress: `2%`
- message: `Target pod "<namespace>/<name>" is unschedulable`

### Disk attach or mount error

If the target pod is in container creation and warning events include:

- `FailedAttachVolume`
- `FailedMount`

then VMOP uses:

- reason: `TargetDiskError`
- progress: `3%`
- message: `Target pod has disk attach error: <event reason>: <event message>`

## VM condition projection

The VM controller projects VMOP migration reasons to the VM `Migrating` condition.

Important messages:

| VMOP reason | VM message |
|---|---|
| `MigrationPending` | `Migration is awaiting start.` |
| `TargetScheduling` | `Migration is in progress: target pod is being scheduled.` |
| `MigrationPrepareTarget`, `TargetPreparing`, `DisksPreparing` | `Migration is in progress: target pod is being scheduled and prepared.` |
| `MigrationTargetReady`, `Syncing`, `SourceSuspended`, `TargetResumed` | `Migration is in progress: source and target are being synchronized.` |

## Practical examples

| Scenario | VMOP phase | Reason | Progress |
|---|---|---|---:|
| Migration object exists but KubeVirt has not started scheduling | `Pending` | `MigrationPending` | `0%` |
| Target pod scheduling starts | `InProgress` | `TargetScheduling` | `2%` |
| Target pod cannot be scheduled | `InProgress` or `Failed` | `TargetUnschedulable` | `2%` |
| Target pod has volume attach errors | `InProgress` or `Failed` | `TargetDiskError` | `3%` |
| Syncing without iteration metrics | `InProgress` | `Syncing` | `10..45%` |
| Syncing with iteration metrics | `InProgress` | `Syncing` | `45..90%` |
| Sync stalls at maximum throttle | `InProgress` or `Failed` | `NotConverging` | dynamic |
| Migration is aborted | `Failed` | `Aborted` | keep current |
| Unknown failure | `Failed` | `Failed` | keep current |
| Source suspended during final handoff | `InProgress` | `SourceSuspended` | `91%` |
| Target resumed | `InProgress` | `TargetResumed` | `92%` |
| Migration succeeds | `Completed` | `Completed` | `100%` |
