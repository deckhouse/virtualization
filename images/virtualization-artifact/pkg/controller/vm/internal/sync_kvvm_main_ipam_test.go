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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonnetwork "github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Main interface spec of a machine moving to the sdn module", func() {
	const (
		name       = "vm"
		namespace  = "vms"
		address    = "10.66.10.14"
		mirrorName = "vm-mirror"
		vmipUID    = types.UID("vm-ip-uid")
	)

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.WithWatch
		vmState    state.VirtualMachineState
		vm         *v1alpha2.VirtualMachine
		kvvm       *virtv1.VirtualMachine
		recorder   *eventrecord.EventRecorderLoggerMock
		events     []string
	)

	BeforeEach(func() {
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: "vm-uid"},
			Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning},
		}
		kvvm = &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{netmanager.AnnoIPAddressCNIRequest: address},
					},
				},
			},
		}
		events = nil
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, reason, _ string) { events = append(events, reason) },
			EventfFunc:      func(_ client.Object, _, reason, _ string, _ ...interface{}) { events = append(events, reason) },
			WithLoggingFunc: func(_ eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	newManagedVMIP := func() *v1alpha2.VirtualMachineIPAddress {
		return &v1alpha2.VirtualMachineIPAddress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-ip",
				Namespace: namespace,
				UID:       vmipUID,
				Labels:    map[string]string{annotations.LabelVirtualMachineUID: "vm-uid"},
			},
			Spec:   v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto},
			Status: v1alpha2.VirtualMachineIPAddressStatus{Address: address},
		}
	}

	// newMirror creates the mirror of the vmip address; vmUID labels it for the machine.
	newMirror := func(vmUID, mirrorAddress string, allocated bool) *sdnapi.IPAddress {
		mirror := newSDNIPAddress(mirrorName, namespace, vmUID, v1alpha2.NetworksTypeClusterNetwork, commonnetwork.MainClusterNetworkName, mirrorAddress, allocated)
		mirror.Spec.Type = sdnapi.IPAddressTypeStatic
		mirror.Spec.Static = &sdnapi.StaticIPAddress{IP: mirrorAddress}
		mirror.SetAnnotations(map[string]string{annotations.AnnMirrorOfVMIP: string(vmipUID)})
		return mirror
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: map[string]string{netmanager.AnnoIPAddressCNIRequest: address},
			},
		}
	}

	withMainInterfaceSpec := func(anno map[string]string) {
		anno[annotations.AnnNetworksSpec] = `[{"id":1,"type":"Main","ifName":"eth0","ipAddressNames":["` + mirrorName + `"],"ipAddress":"` + address + `"}]`
	}

	getKVVM := func() *virtv1.VirtualMachine {
		obj := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), obj)).To(Succeed())
		return obj
	}

	getKVVMI := func() *virtv1.VirtualMachineInstance {
		obj := &virtv1.VirtualMachineInstance{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(kvvm), obj)).To(Succeed())
		return obj
	}

	addMainInterfaceSpec := func() bool {
		h := &SyncKvvmHandler{client: fakeClient, recorder: recorder}
		written, err := h.addMainInterfaceSpec(ctx, vmState, getKVVM())
		Expect(err).NotTo(HaveOccurred())
		return written
	}

	Context("SyncKvvmHandler", func() {
		It("writes the Main interface spec into the template next to the legacy annotation", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", address, true))

			Expect(addMainInterfaceSpec()).To(BeTrue())

			templateAnno := getKVVM().Spec.Template.ObjectMeta.Annotations
			Expect(templateAnno).To(HaveKeyWithValue(netmanager.AnnoIPAddressCNIRequest, address))
			Expect(templateAnno[annotations.AnnNetworksSpec]).To(ContainSubstring(`"type":"Main"`))
			Expect(templateAnno[annotations.AnnNetworksSpec]).To(ContainSubstring(mirrorName))
			Expect(events).To(ContainElement(v1alpha2.ReasonMainIPAMMigrated))
		})

		It("writes it into the running instance too", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newKVVMI(), newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", address, true))

			Expect(addMainInterfaceSpec()).To(BeTrue())

			// The instance is what a live-migration target pod is created from.
			instanceAnno := getKVVMI().Annotations
			Expect(instanceAnno).To(HaveKeyWithValue(netmanager.AnnoIPAddressCNIRequest, address))
			Expect(instanceAnno[annotations.AnnNetworksSpec]).To(ContainSubstring(mirrorName))
		})

		It("writes the instance of a migrated machine that has no Main interface spec yet", func() {
			withMainInterfaceSpec(kvvm.Spec.Template.ObjectMeta.Annotations)
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newKVVMI(), newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", address, true))

			Expect(addMainInterfaceSpec()).To(BeFalse())

			Expect(getKVVMI().Annotations[annotations.AnnNetworksSpec]).To(ContainSubstring(mirrorName))
			Expect(events).To(BeEmpty())
		})

		It("leaves a migrated machine alone", func() {
			withMainInterfaceSpec(kvvm.Spec.Template.ObjectMeta.Annotations)
			kvvmi := newKVVMI()
			withMainInterfaceSpec(kvvmi.Annotations)
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, kvvmi, newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", address, true))
			before := getKVVMI().ResourceVersion

			Expect(addMainInterfaceSpec()).To(BeFalse())

			Expect(getKVVMI().ResourceVersion).To(Equal(before))
			Expect(events).To(BeEmpty())
		})

		It("leaves a machine without a Main network alone", func() {
			vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cn"}}
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork())

			Expect(addMainInterfaceSpec()).To(BeFalse())
			Expect(commonnetwork.KVVMHasMainInterfaceSpec(getKVVM())).To(BeFalse())
		})

		It("leaves a machine alone where the sdn module does not serve the Main network", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newManagedVMIP(), newMirror("vm-uid", address, true))

			Expect(addMainInterfaceSpec()).To(BeFalse())
			Expect(commonnetwork.KVVMHasMainInterfaceSpec(getKVVM())).To(BeFalse())
		})

		It("waits for the mirror to exist", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP())

			Expect(addMainInterfaceSpec()).To(BeFalse())
			Expect(commonnetwork.KVVMHasMainInterfaceSpec(getKVVM())).To(BeFalse())
		})

		It("waits for the mirror to be allocated", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", address, false))

			Expect(addMainInterfaceSpec()).To(BeFalse())
			Expect(commonnetwork.KVVMHasMainInterfaceSpec(getKVVM())).To(BeFalse())
		})

		It("waits for the mirror of the vmip address", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("vm-uid", "10.66.10.99", true))

			Expect(addMainInterfaceSpec()).To(BeFalse())
			Expect(commonnetwork.KVVMHasMainInterfaceSpec(getKVVM())).To(BeFalse())
		})

		It("moves a machine pinned to an IPAddress onto the address it named", func() {
			pinned := newSDNIPAddress("user-pinned-ip", namespace, "", v1alpha2.NetworksTypeClusterNetwork, commonnetwork.MainClusterNetworkName, "10.66.10.99", true)
			vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "user-pinned-ip"}}
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), pinned)

			Expect(addMainInterfaceSpec()).To(BeTrue())
			Expect(getKVVM().Spec.Template.ObjectMeta.Annotations[annotations.AnnNetworksSpec]).To(ContainSubstring("user-pinned-ip"))
		})

		It("leaves the instance alone while it runs on another address", func() {
			// The machine has been repinned but not restarted: the instance keeps
			// describing the pod that exists, or a live migration would move it.
			pinned := newSDNIPAddress("user-pinned-ip", namespace, "", v1alpha2.NetworksTypeClusterNetwork, commonnetwork.MainClusterNetworkName, "10.66.10.99", true)
			vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain, IPAddressName: "user-pinned-ip"}}
			vm.Status.IPAddress = address
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newKVVMI(), newMainPrimaryCNINetwork(), pinned)

			addMainInterfaceSpec()

			Expect(getKVVMI().Annotations).NotTo(HaveKey(annotations.AnnNetworksSpec))
		})
	})

	Context("IPAddressHandler", func() {
		handle := func() {
			_, err := NewIPAddressHandler(fakeClient, recorder).Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())
		}

		getMirror := func() *sdnapi.IPAddress {
			obj := &sdnapi.IPAddress{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: mirrorName, Namespace: namespace}, obj)).To(Succeed())
			return obj
		}

		It("labels the mirror of a vmip the machine names explicitly", func() {
			// A user-created vmip carries no machine label, and neither does its mirror.
			userVMIP := newManagedVMIP()
			userVMIP.Labels = nil
			userVMIP.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
			vm.Spec.VirtualMachineIPAddress = userVMIP.Name
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), userVMIP, newMirror("", address, true))

			handle()

			Expect(getMirror().GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
			Expect(events).To(BeEmpty())
		})

		It("does not label the mirror before it is allocated", func() {
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP(), newMirror("", address, false))

			handle()

			Expect(getMirror().GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
		})

		// The machine ran on its own IPAddress and the owner named a vmip with another
		// address: the Main interface spec has to follow the vmip, and the own IPAddress
		// stays only while the machine runs on it.
		Context("a machine moving from its own IPAddress to a vmip", func() {
			const ownAddress = "10.66.12.107"

			newOwnIPAddress := func() *sdnapi.IPAddress {
				return newSDNIPAddress("vm-own", namespace, "vm-uid", v1alpha2.NetworksTypeClusterNetwork, commonnetwork.MainClusterNetworkName, ownAddress, true)
			}

			setup := func(runningAddress string) {
				userVMIP := newManagedVMIP()
				userVMIP.Labels = nil
				userVMIP.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
				vm.Spec.VirtualMachineIPAddress = userVMIP.Name
				vm.Status.IPAddress = runningAddress
				withMainInterfaceSpec(kvvm.Spec.Template.ObjectMeta.Annotations)
				fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), userVMIP, newMirror("", address, true), newOwnIPAddress())
			}

			It("resolves the mirror of the vmip and keeps both while the machine runs on its own address", func() {
				setup(ownAddress)

				handle()

				Expect(getMirror().GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
				ipa, err := commonnetwork.GetMainSDNIPAddress(ctx, fakeClient, vm)
				Expect(err).NotTo(HaveOccurred())
				Expect(ipa).NotTo(BeNil())
				Expect(ipa.GetName()).To(Equal(mirrorName), "the next pod has to come up on the address of the vmip")
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "vm-own", Namespace: namespace}, &sdnapi.IPAddress{})).To(Succeed())
			})

			It("deletes the own IPAddress once the machine runs on the vmip address", func() {
				setup(address)

				handle()

				Expect(getMirror().GetLabels()).To(HaveKeyWithValue(annotations.LabelVirtualMachineUID, "vm-uid"))
				err := fakeClient.Get(ctx, types.NamespacedName{Name: "vm-own", Namespace: namespace}, &sdnapi.IPAddress{})
				Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "nothing uses the former address any more")
			})
		})

		It("reports a foreign IPAddress on the address instead of taking it", func() {
			foreign := newMirror("", address, true)
			foreign.SetAnnotations(nil)
			fakeClient, _, vmState = setupEnvironment(vm, kvvm, newMainPrimaryCNINetwork(), newManagedVMIP(), foreign)

			result, err := NewIPAddressHandler(fakeClient, recorder).Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			// Labeling it would hand the user resource to the Main lookup of the
			// machine and to the collector of orphaned addresses.
			Expect(getMirror().GetLabels()).NotTo(HaveKey(annotations.LabelVirtualMachineUID))
			Expect(events).To(ContainElement(v1alpha2.ReasonMainIPAMMigrationBlocked))
			Expect(result.RequeueAfter).To(Equal(addressInUseRetry))
		})
	})
})
