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

package vm_test

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
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	vmevent "github.com/deckhouse/virtualization-controller/pkg/audit/events/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	v1alpha "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	var vm *v1alpha.VirtualMachine
	var vd *v1alpha.VirtualDisk
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

		vm = &v1alpha.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "test-vm", Namespace: "test", UID: "0000-0000-4567"},
			Spec: v1alpha.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha.BlockDeviceSpecRef{
					{Kind: v1alpha.VirtualDiskKind, Name: "test-disk"},
					{Kind: v1alpha.VirtualImageKind, Name: "test-image"},
				},
			},
			Status: v1alpha.VirtualMachineStatus{
				Node: "test-node",
				GuestOSInfo: virtv1.VirtualMachineInstanceGuestOSInfo{
					Name: "test-os",
				},
			},
		}

		vd = &v1alpha.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "test-disk", Namespace: "test", UID: "0000-0000-4567"},
			Status: v1alpha.VirtualDiskStatus{
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
				GetVMInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vm, !args.shouldLostVM, nil
						},
					}
				},
				GetVDInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vd, !args.shouldLostVD, nil
						},
					}
				},
				GetNodeInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return node, !args.shouldLostNode, nil
						},
					}
				},
			}

			eventLog := vmevent.VMAccess{
				Event:        event,
				InformerList: informerList,
				TTLCache:     ttlCache,
			}

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

			Expect(eventLog.EventLog.Type).To(Equal("Access to VM"))
			Expect(eventLog.EventLog.Level).To(Equal("info"))
			Expect(eventLog.EventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.EventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.EventLog.Uid).To(Equal("0000-0000-0000"))
			Expect(eventLog.EventLog.ActionType).To(Equal("get"))

			if args.isRequestReceived {
				Expect(eventLog.EventLog.OperationResult).To(Equal("unknown"))
			} else {
				Expect(eventLog.EventLog.OperationResult).To(Equal("allow"))
			}

			if args.shouldLostVM || args.shouldLostVD || args.shouldLostNode {
				return
			}

			Expect(eventLog.EventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.EventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.EventLog.FirmwareVersion).To(Equal("unknown"))
			Expect(eventLog.EventLog.RequestSubject).To(Equal("test-user"))

			if !args.shouldLostNode {
				Expect(eventLog.EventLog.NodeNetworkAddress).To(Equal("127.0.0.1"))
			} else {
				Expect(eventLog.EventLog.NodeNetworkAddress).To(Equal("unknown"))
			}

			if !args.shouldLostVD {
				Expect(eventLog.EventLog.StorageClasses).To(Equal("test-storageclass"))
			} else {
				Expect(eventLog.EventLog.StorageClasses).To(Equal("unknown"))
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
			expectedName:      "Access to VM via serial console",
			customSubresource: "console",
		}),
		Entry("VM Access with RequestReceived shouldn't contain decision and fill without errors", vmAccessTestArgs{
			expectedName:      "Request Access to VM via serial console",
			customSubresource: "console",
			isRequestReceived: true,
		}),
		Entry("VM Access by Console event should filled without errors", vmAccessTestArgs{
			expectedName:      "Access to VM via serial console",
			customSubresource: "console",
		}),
		Entry("VM Access by VNC event should filled without errors", vmAccessTestArgs{
			expectedName:      "Access to VM via VNC",
			customSubresource: "vnc",
		}),
		Entry("VM Access by Portforward event should filled without errors", vmAccessTestArgs{
			expectedName:      "Access to VM via portforward",
			customSubresource: "portforward",
		}),
		Entry("VM Access with losted VM event should filled without errors", vmAccessTestArgs{
			expectedName:      "Access to VM via serial console",
			customSubresource: "console",
			shouldLostVM:      true,
		}),
		Entry("VM Access with losted VD and Node event should filled without errors", vmAccessTestArgs{
			expectedName:      "Access to VM via serial console",
			customSubresource: "console",
			shouldLostVD:      true,
			shouldLostNode:    true,
		}),
	)
})
