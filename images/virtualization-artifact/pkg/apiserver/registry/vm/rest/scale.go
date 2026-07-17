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

package rest

import (
	"context"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

// Labels carried by a VM's virt-launcher pod. virtualization-api runs without
// kube-api-rewriter, so it sees the rebranded internal.virtualization.deckhouse.io
// domain as-is rather than the upstream kubevirt.io constants. There is no shared
// constant for it yet, so they are declared here.
const (
	// launcherAppLabel selects virt-launcher pods; the value is always "virt-launcher".
	launcherAppLabel = "kubevirt.internal.virtualization.deckhouse.io"
	// vmNameLabel carries the owning VM's name on the virt-launcher pod.
	vmNameLabel = "vm.kubevirt.internal.virtualization.deckhouse.io/name"
)

// ScaleREST implements the scale subresource for VirtualMachine.
//
// It exists so the stock VPA recommender can discover a VM's virt-launcher pod:
// VPA fetches the target through the polymorphic scale client and reads only
// scale.status.selector (see autoscaler pkg/target/fetcher.go), so Get returns a
// Scale whose selector matches the launcher pod. spec/status.replicas are a
// constant 1 to keep the scale contract valid — a VM is a single instance and has
// no replica count to change, so the subresource is intentionally read-only.
type ScaleREST struct {
	vmLister virtlisters.VirtualMachineLister
}

var (
	_ rest.Storage                  = &ScaleREST{}
	_ rest.Getter                   = &ScaleREST{}
	_ rest.GroupVersionKindProvider = &ScaleREST{}
)

func NewScaleREST(vmLister virtlisters.VirtualMachineLister) *ScaleREST {
	return &ScaleREST{vmLister: vmLister}
}

// New implements rest.Storage interface.
func (r *ScaleREST) New() runtime.Object {
	return &autoscalingv1.Scale{}
}

// Destroy implements rest.Storage interface.
func (r *ScaleREST) Destroy() {}

// GroupVersionKind tells the endpoints installer to serve this subresource as an
// autoscaling/v1 Scale, regardless of the parent (subresources.deckhouse.io) group.
// The polymorphic scale client used by VPA expects exactly this kind.
func (r *ScaleREST) GroupVersionKind(schema.GroupVersion) schema.GroupVersionKind {
	return autoscalingv1.SchemeGroupVersion.WithKind("Scale")
}

// Get implements rest.Getter interface.
func (r *ScaleREST) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	vm, err := r.vmLister.VirtualMachines(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(subresources.Resource("virtualmachines"), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}

	// Matches the VM's virt-launcher pod(s). Same selector the controller uses to
	// list launcher pods; VPA parses it to find the pod that carries the metrics.
	selector := labels.SelectorFromSet(labels.Set{
		launcherAppLabel: "virt-launcher",
		vmNameLabel:      vm.GetName(),
	}).String()

	return &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:              vm.GetName(),
			Namespace:         vm.GetNamespace(),
			UID:               vm.GetUID(),
			ResourceVersion:   vm.GetResourceVersion(),
			CreationTimestamp: vm.GetCreationTimestamp(),
		},
		Spec:   autoscalingv1.ScaleSpec{Replicas: 1},
		Status: autoscalingv1.ScaleStatus{Replicas: 1, Selector: selector},
	}, nil
}
