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

package freezelog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// record writes one line through For(ctx) and decodes it, so the assertions read the record exactly as
// it lands in the log.
func record(t *testing.T, ctx context.Context, attrs []any) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	ctx = logger.ToContext(ctx, slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	For(ctx).Info("freeze record", attrs...)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode the emitted record %q: %v", buf.String(), err)
	}
	return got
}

// A record has to name the freeze subsystem, so freeze lines can be pulled out of a controller log that
// is mostly phase transitions.
func TestFor_TagsTheSubsystem(t *testing.T) {
	got := record(t, context.Background(), nil)

	if got["subsystem"] != "fsfreeze" {
		t.Errorf("subsystem = %v, want fsfreeze", got["subsystem"])
	}
	if got["msg"] != "freeze record" {
		t.Errorf("msg = %v", got["msg"])
	}
}

// The whole point of taking the logger from the context: it carries what controller-runtime put there
// for this request. Read off a controller-level logger instead, a freeze record cannot be tied to the
// snapshot it belongs to.
func TestFor_KeepsTheReconcileContext(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil)).With(
		logger.SlogNamespace("ns-1"), logger.SlogName("vms-1"),
	)
	ctx := logger.ToContext(context.Background(), base)

	For(ctx).Info("freeze record")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["namespace"] != "ns-1" || got["name"] != "vms-1" {
		t.Errorf("record = %v, want it to carry namespace=ns-1 name=vms-1", got)
	}
}

// "There is no instance to freeze" and "the instance is not frozen" lead to opposite decisions, so they
// must not look alike in the log.
func TestState_DistinguishesAMissingInstanceFromAThawedOne(t *testing.T) {
	missing := record(t, context.Background(), State(nil))
	if missing["kvvmiPresent"] != false {
		t.Errorf("kvvmiPresent = %v, want false", missing["kvvmiPresent"])
	}
	if _, ok := missing["fsFreezeStatus"]; ok {
		t.Error("a missing instance reported an fsFreezeStatus; there is no instance to have one")
	}

	thawed := record(t, context.Background(), State(&virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vm"},
		Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
	}))
	if thawed["kvvmiPresent"] != true {
		t.Errorf("kvvmiPresent = %v, want true", thawed["kvvmiPresent"])
	}
	if thawed["fsFreezeStatus"] != "" {
		t.Errorf("fsFreezeStatus = %q, want the empty string a thawed guest reports", thawed["fsFreezeStatus"])
	}
	if thawed["requestPending"] != false {
		t.Errorf("requestPending = %v, want false", thawed["requestPending"])
	}
}

// A freeze that was asked for but not yet confirmed is the state the parent waits on, and the one a
// child must not mistake for "thawed". Both halves have to be visible.
func TestState_ReportsAPendingRequest(t *testing.T) {
	got := record(t, context.Background(), State(&virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "vm",
			Annotations: map[string]string{annotations.AnnVMFilesystemRequest: "freeze"},
		},
		Status: virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
	}))

	if got["requestPending"] != true {
		t.Errorf("requestPending = %v, want true", got["requestPending"])
	}
	if got["request"] != "freeze" {
		t.Errorf("request = %v, want freeze", got["request"])
	}
	if got["fsFreezeStatus"] != "" {
		t.Errorf("fsFreezeStatus = %q, want empty: the guest has not confirmed yet", got["fsFreezeStatus"])
	}
}

func TestState_ReportsAFrozenGuest(t *testing.T) {
	got := record(t, context.Background(), State(&virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "vm"},
		Status: virtv1.VirtualMachineInstanceStatus{
			Phase: virtv1.Running, FSFreezeStatus: "frozen",
		},
	}))

	if got["fsFreezeStatus"] != "frozen" {
		t.Errorf("fsFreezeStatus = %v, want frozen", got["fsFreezeStatus"])
	}
	if got["kvvmiPhase"] != "Running" {
		t.Errorf("kvvmiPhase = %v, want Running", got["kvvmiPhase"])
	}
}

// Attrs carries the caller's own attributes alongside the observed state.
func TestAttrs_KeepsBothHalves(t *testing.T) {
	got := record(t, context.Background(), Attrs(nil, slog.String("verdict", "failed")))

	if got["kvvmiPresent"] != false {
		t.Errorf("kvvmiPresent = %v, want the state half to survive", got["kvvmiPresent"])
	}
	if got["verdict"] != "failed" {
		t.Errorf("verdict = %v, want the caller's half to survive", got["verdict"])
	}
}
