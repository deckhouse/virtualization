/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package freezelog records what happens to a guest filesystem freeze during a snapshot.
//
// The freeze is one shared, short-lived resource: a VirtualMachineSnapshot takes it once and every
// VirtualDiskSnapshot under it captures while it is held. When a disk reports that the machine is running
// and not frozen, the question is always the same — was the freeze never taken, released early, or
// dropped by the guest — and nothing in the log could answer it: the two controllers recorded only their
// phase transitions, so a consistency failure left no trace of the freeze at all.
//
// Every record carries the state actually read off the VirtualMachineInstance, not the controller's
// conclusion about it, so records written by the parent and by each child can be compared directly.
package freezelog

import (
	"context"
	"log/slog"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// For returns the reconcile-scoped logger tagged as a freeze record. It is taken from the context, which
// is where controller-runtime puts the logger it built for this request — so the record names the object
// and the reconcile it belongs to, which a controller-level logger does not.
func For(ctx context.Context) *slog.Logger {
	return logger.FromContext(ctx).With(slog.String("subsystem", "fsfreeze"))
}

// State describes the guest filesystem freeze as the VirtualMachineInstance reports it. A nil instance is
// reported as such rather than omitted: "there is no instance to freeze" and "the instance is not frozen"
// lead to opposite decisions and must not look alike in the log.
func State(kvvmi *virtv1.VirtualMachineInstance) []any {
	if kvvmi == nil {
		return []any{slog.Bool("kvvmiPresent", false)}
	}

	request, requested := kvvmi.Annotations[annotations.AnnVMFilesystemRequest]

	return []any{
		slog.Bool("kvvmiPresent", true),
		slog.String("kvvmi", kvvmi.Name),
		slog.String("kvvmiPhase", string(kvvmi.Status.Phase)),
		// Empty means thawed. Recorded verbatim so a freeze that lapsed is distinguishable from one that
		// was never requested.
		slog.String("fsFreezeStatus", kvvmi.Status.FSFreezeStatus),
		slog.Bool("requestPending", requested),
		slog.String("request", request),
	}
}

// Attrs prefixes a record's own attributes with the freeze state.
func Attrs(kvvmi *virtv1.VirtualMachineInstance, attrs ...any) []any {
	return append(State(kvvmi), attrs...)
}
