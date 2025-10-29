/*
Copyright 2025 Flant JSC

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
	"os"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type vmAccessTestArgs struct {
	eventVerb          string
	expectedName       string
	shouldLostVM       bool
	shouldLostVD       bool
	shouldLostNode     bool
	customObjectRef    *audit.ObjectReference
	isRequestReceived  bool
	customSubresource  string
	customObjectRefNil bool
	shouldFailMatch    bool
}

var _ = Describe("VMOP Events", func() {
	var event *audit.Event
	var vm *v1alpha2.VirtualMachine
	var vd *v1alpha2.VirtualDisk
	var node *corev1.Node

	currentTime := time.Now()

	BeforeEach(func() {
		event = &audit.Event{
			TypeMeta:                 metav1.TypeMeta{},
			Level:                    audit.LevelRequestResponse,
			AuditID:                  "0000-0000-0000",
			Stage:                    audit.StageResponseComplete,
			Verb:                     "get",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				APIGroup:    "subresources.virtualization.deckhouse.io",
				Resource:    "virtualmachines",
				Namespace:   "test",
				Name:        "virt-launcher-test-vm",
				Subresource: "console",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "test", UID: "0000-0000-4567"},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Kind: v1alpha2.VirtualDiskKind, Name: "test-disk"},
					{Kind: v1alpha2.VirtualImageKind, Name: "test-image"},
				},
			},
			Status: v1alpha2.VirtualMachineStatus{
				Node: "test-node",
				GuestOSInfo: virtv1.VirtualMachineInstanceGuestOSInfo{
					Name: "test-os",
				},
				Versions: v1alpha2.Versions{
					Qemu:    "9.9.9",
					Libvirt: "1.1.1",
				},
			},
		}

		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "test-disk", Namespace: "test", UID: "0000-0000-4567"},
			Status: v1alpha2.VirtualDiskStatus{
				StorageClassName: "test-storageclass",
			},
		}

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "127.0.0.1"}},
			},
		}
	})

	DescribeTable("Checking VMOP events",
		func(args vmAccessTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetVMInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vm, !args.shouldLostVM, nil
						},
					}
				},
				GetVDInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vd, !args.shouldLostVD, nil
						},
					}
				},
				GetNodeInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return node, !args.shouldLostNode, nil
						},
					}
				},
			}

			eventLoggerOptions := events.EventLoggerOptionsMock{
				GetEventFunc: func() *audit.Event {
					return event
				},
				GetInformerListFunc: func() events.InformerList {
					return informerList
				},
				GetTTLCacheFunc: func() events.TTLCache {
					return ttlCache
				},
			}

			eventLog := NewVMAccess(&eventLoggerOptions)

			if args.eventVerb != "" {
				event.Verb = args.eventVerb
			}

			if args.customObjectRef != nil {
				event.ObjectRef = args.customObjectRef
			}

			if args.customObjectRefNil {
				event.ObjectRef = nil
			}

			if args.customSubresource != "" {
				event.ObjectRef.Subresource = args.customSubresource
			}

			if args.isRequestReceived {
				event.Stage = audit.StageRequestReceived
				event.Annotations = map[string]string{}
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Access to VM"))
			Expect(eventLog.eventLog.Level).To(Equal("info"))
			Expect(eventLog.eventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.ActionType).To(Equal("get"))

			if args.isRequestReceived {
				Expect(eventLog.eventLog.OperationResult).To(Equal("unknown"))
			} else {
				Expect(eventLog.eventLog.OperationResult).To(Equal("allow"))
			}

			if args.shouldLostVM || args.shouldLostVD || args.shouldLostNode {
				return
			}

			Expect(eventLog.eventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.eventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.eventLog.QemuVersion).To(Equal("9.9.9"))
			Expect(eventLog.eventLog.LibvirtVersion).To(Equal("1.1.1"))
			Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))

			if !args.shouldLostNode {
				Expect(eventLog.eventLog.NodeNetworkAddress).To(Equal("127.0.0.1"))
			} else {
				Expect(eventLog.eventLog.NodeNetworkAddress).To(Equal("unknown"))
			}

			if !args.shouldLostVD {
				Expect(eventLog.eventLog.StorageClasses).To(Equal("test-storageclass"))
			} else {
				Expect(eventLog.eventLog.StorageClasses).To(Equal("unknown"))
			}

			Expect(eventLog.ShouldLog()).To(BeTrue())

			// Temporary redirect stdout to /dev/null
			defer func(stdout *os.File) {
				os.Stdout = stdout
			}(os.Stdout)
			os.Stdout = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

			err = eventLog.Log()
			Expect(err).To(BeNil())
		},
		Entry("VM Access event should failed match if objectRef is nil", vmAccessTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("VM Access event should failed match if verb is not get", vmAccessTestArgs{
			eventVerb:       "create",
			shouldFailMatch: true,
		}),
		Entry("VM Access event should failed match if resource is not virtualmachines", vmAccessTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations"},
			shouldFailMatch: true,
		}),
		Entry("VM Access event should failed match if apigroup is not subresource", vmAccessTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachines", APIGroup: "virtualization.deckhouse.io"},
			shouldFailMatch: true,
		}),
		Entry("VM Access with ResponseComplete should contain decision and fill without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been finished via console by 'test-user'",
			customSubresource: "console",
		}),
		Entry("VM Access with RequestReceived shouldn't contain decision and fill without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been initiated via console by 'test-user'",
			customSubresource: "console",
			isRequestReceived: true,
		}),
		Entry("VM Access by Console event should filled without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been finished via console by 'test-user'",
			customSubresource: "console",
		}),
		Entry("VM Access by VNC event should filled without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been finished via vnc by 'test-user'",
			customSubresource: "vnc",
		}),
		Entry("VM Access by Portforward event should filled without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been finished via portforward by 'test-user'",
			customSubresource: "portforward",
		}),
		Entry("VM Access with losted VM event should filled without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'virt-launcher-test-vm' connection has been finished via console by 'test-user'",
			customSubresource: "console",
			shouldLostVM:      true,
		}),
		Entry("VM Access with losted VD and Node event should filled without errors", vmAccessTestArgs{
			expectedName:      "Virtual machine 'test-vm' connection has been finished via console by 'test-user'",
			customSubresource: "console",
			shouldLostVD:      true,
			shouldLostNode:    true,
		}),
	)
})
