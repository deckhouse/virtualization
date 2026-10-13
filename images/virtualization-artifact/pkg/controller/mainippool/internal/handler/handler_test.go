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

package handler

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
)

// newRecordingClient returns a fake client that records the apply patches instead of
// applying them: merging the applied fields is the job of the API server.
func newRecordingClient(applied *[]client.Object, objs ...client.Object) client.Client {
	scheme := apiruntime.NewScheme()
	Expect(sdnapi.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
				Expect(patch).To(Equal(client.Apply))
				*applied = append(*applied, obj.DeepCopyObject().(client.Object))
				return nil
			},
		}).Build()
}

func newMainClusterNetwork(networkType string, poolRef *sdnapi.IPAddressPoolRef) *sdnapi.ClusterNetwork {
	cn := &sdnapi.ClusterNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: network.MainClusterNetworkName},
		Spec:       sdnapi.ClusterNetworkSpec{Type: networkType},
	}
	if poolRef != nil {
		cn.Spec.IPAM = &sdnapi.IPAMSpec{IPAddressPoolRef: poolRef}
	}
	return cn
}

var _ = Describe("PoolHandler", func() {
	It("applies a range per virtualMachineCIDRs entry", func() {
		var applied []client.Object
		h := NewPoolHandler(newRecordingClient(&applied), []string{"10.66.10.0/24", "10.66.20.0/24"})

		Expect(h.Handle(context.Background())).To(Succeed())

		Expect(applied).To(HaveLen(1))
		pool, ok := applied[0].(*sdnapi.ClusterIPAddressPool)
		Expect(ok).To(BeTrue())
		Expect(pool.Name).To(Equal(network.MainIPAddressPoolName))
		Expect(pool.Spec.Pools).To(Equal([]sdnapi.IPAddressPoolRange{{Network: "10.66.10.0/24"}, {Network: "10.66.20.0/24"}}))
	})
})

var _ = Describe("PoolHandler on a pool being deleted", func() {
	It("reports the deletion instead of keeping the pool", func() {
		scheme := apiruntime.NewScheme()
		Expect(sdnapi.AddToScheme(scheme)).To(Succeed())
		cl := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, _ client.Patch, _ ...client.PatchOption) error {
				now := metav1.Now()
				obj.SetDeletionTimestamp(&now)
				return nil
			},
		}).Build()

		err := NewPoolHandler(cl, []string{"10.66.10.0/24"}).Handle(context.Background())

		Expect(err).To(MatchError(ErrPoolTerminating))
	})
})

var _ = Describe("PoolRefHandler", func() {
	ownPool := &sdnapi.IPAddressPoolRef{Kind: sdnapi.ClusterIPAddressPoolKind, Name: network.MainIPAddressPoolName}

	handle := func(objs ...client.Object) []client.Object {
		var applied []client.Object
		Expect(NewPoolRefHandler(newRecordingClient(&applied, objs...)).Handle(context.Background())).To(Succeed())
		return applied
	}

	It("binds the pool to the main network without one", func() {
		applied := handle(newMainClusterNetwork(sdnapi.ClusterNetworkTypePrimaryCNI, nil))

		Expect(applied).To(HaveLen(1))
		ref := applied[0].(*unstructured.Unstructured)
		Expect(ref.GetName()).To(Equal(network.MainClusterNetworkName))
		Expect(ref.Object["spec"]).To(Equal(map[string]any{
			"ipam": map[string]any{"ipAddressPoolRef": map[string]any{
				"kind": sdnapi.ClusterIPAddressPoolKind,
				"name": network.MainIPAddressPoolName,
			}},
		}), "the apply carries the reference only, not spec.type")
	})

	It("leaves the network alone once it has the pool", func() {
		Expect(handle(newMainClusterNetwork(sdnapi.ClusterNetworkTypePrimaryCNI, ownPool))).To(BeEmpty())
	})

	It("leaves a pool somebody else put there", func() {
		other := &sdnapi.IPAddressPoolRef{Kind: sdnapi.ClusterIPAddressPoolKind, Name: "custom"}
		Expect(handle(newMainClusterNetwork(sdnapi.ClusterNetworkTypePrimaryCNI, other))).To(BeEmpty())
	})

	It("leaves a main network that is not PrimaryCNI", func() {
		Expect(handle(newMainClusterNetwork("Access", nil))).To(BeEmpty())
	})

	It("does nothing without the main network", func() {
		Expect(handle()).To(BeEmpty())
	})
})
