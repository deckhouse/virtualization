/*
Copyright 2024 Flant JSC

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

package vm

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
)

const (
	GCVMMigrationControllerName  = "vmi-migration-gc-controller"
	GCCompletedPodControllerName = "completed-pod-gc-controller"
)

func SetupGCMigrations(
	mgr manager.Manager,
	log *log.Logger,
	gcSettings config.BaseGcSettings,
) error {
	mgrClient := mgr.GetClient()
	vmimGCMgr := newVMIMGCManager(mgrClient, gcSettings.TTL.Duration, 10)
	source, err := gc.NewCronSource(gcSettings.Schedule, vmimGCMgr, log.With("resource", "vmi-migration"))
	if err != nil {
		return err
	}
	return gc.SetupGcController(GCVMMigrationControllerName,
		mgr,
		log,
		source,
		vmimGCMgr,
	)
}

func SetupGCCompletedPods(
	mgr manager.Manager,
	log *log.Logger,
	gcSettings config.BaseGcSettings,
) error {
	podGCMgr := newCompletedPodGCManager(mgr.GetClient(), 10)
	source, err := gc.NewCronSource(gcSettings.Schedule, podGCMgr, log.With("resource", "pod"))
	if err != nil {
		return err
	}
	return gc.SetupGcController(GCCompletedPodControllerName,
		mgr,
		log,
		source,
		podGCMgr,
	)
}

func newVMIMGCManager(client client.Client, ttl time.Duration, max int) *vmimGCManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	if max == 0 {
		max = 10
	}
	return &vmimGCManager{
		client: client,
		ttl:    ttl,
		max:    max,
	}
}

type vmimGCManager struct {
	client client.Client
	ttl    time.Duration
	max    int
}

func (m *vmimGCManager) New() client.Object {
	return &virtv1.VirtualMachineInstanceMigration{}
}

func (m *vmimGCManager) ShouldBeDeleted(obj client.Object) bool {
	vmim, ok := obj.(*virtv1.VirtualMachineInstanceMigration)
	if !ok {
		return false
	}
	return vmiMigrationIsFinal(vmim)
}

func (m *vmimGCManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	vmimList := &virtv1.VirtualMachineInstanceMigrationList{}
	err := m.client.List(ctx, vmimList)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(vmimList.Items))
	for _, vmim := range vmimList.Items {
		objs = append(objs, &vmim)
	}

	result := gc.DefaultFilter(objs, m.ShouldBeDeleted, m.ttl, m.getIndex, m.max, now)

	return result, nil
}

func (m *vmimGCManager) getIndex(obj client.Object) string {
	vmim, ok := obj.(*virtv1.VirtualMachineInstanceMigration)
	if !ok {
		return ""
	}
	return types.NamespacedName{Namespace: vmim.GetNamespace(), Name: vmim.Spec.VMIName}.String()
}

func vmiMigrationIsFinal(migration *virtv1.VirtualMachineInstanceMigration) bool {
	return migration.Status.Phase == virtv1.MigrationFailed || migration.Status.Phase == virtv1.MigrationSucceeded
}

type completedPodGCmanager struct {
	client client.Client
	max    int
}

func newCompletedPodGCManager(client client.Client, max int) *completedPodGCmanager {
	if max == 0 {
		max = 10
	}
	return &completedPodGCmanager{
		client: client,
		max:    max,
	}
}

func (m *completedPodGCmanager) New() client.Object {
	return &corev1.Pod{}
}

func (m *completedPodGCmanager) ShouldBeDeleted(obj client.Object) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}

	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func (m *completedPodGCmanager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	podList := &corev1.PodList{}
	err := m.client.List(ctx, podList, client.MatchingLabels{
		"kubevirt.internal.virtualization.deckhouse.io": "virt-launcher",
	})
	if err != nil {
		return nil, err
	}

	var objs []client.Object
	for _, pod := range podList.Items {
		if m.ShouldBeDeleted(&pod) {
			objs = append(objs, &pod)
		}
	}

	result := gc.KeepLastRemoveOld(objs, m.getIndex, m.max, now)

	return result, nil
}

func (m *completedPodGCmanager) getIndex(obj client.Object) string {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return ""
	}
	owner := metav1.GetControllerOf(pod)
	if owner != nil {
		return string(owner.UID)
	}
	return ""
}
