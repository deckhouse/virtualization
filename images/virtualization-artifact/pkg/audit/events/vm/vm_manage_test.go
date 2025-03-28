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

type vmManageTestArgs struct {
	EventURI           string
	EventVerb          string
	ExpectedName       string
	ExpectedLevel      string
	ExpectedActionType string
	ShouldLostVM       bool
	ShouldLostVD       bool
	ShouldLostNode     bool
	CustomObjectRef    *audit.ObjectReference
	CustomObjectRefNil bool
	CustomStage        audit.Stage
	ShouldFailMatch    bool
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
			RequestURI:               "/",
			Verb:                     "create",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "virtualmachines",
				Namespace: "test",
				Name:      "test-vm",
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
		func(args vmManageTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetVMInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vm, !args.ShouldLostVM, nil
						},
					}
				},
				GetVDInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vd, !args.ShouldLostVD, nil
						},
					}
				},
				GetNodeInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return node, !args.ShouldLostNode, nil
						},
					}
				},
			}

			eventLog := vmevent.VMManage{
				Event:        event,
				InformerList: informerList,
				TTLCache:     ttlCache,
			}

			if args.EventVerb != "" {
				event.Verb = args.EventVerb
			}

			if args.EventURI != "" {
				event.RequestURI = args.EventURI
			}

			if args.CustomObjectRef != nil {
				event.ObjectRef = args.CustomObjectRef
			}

			if args.CustomObjectRefNil {
				event.ObjectRef = nil
			}

			if args.CustomStage != "" {
				event.Stage = args.CustomStage
			}

			if args.ShouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.ShouldLostVM {
				err := eventLog.Fill()
				Expect(err).To(BeNil())
				return
			}

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.EventLog.Type).To(Equal("Manage VM"))
			Expect(eventLog.EventLog.Level).To(Equal(args.ExpectedLevel))
			Expect(eventLog.EventLog.Name).To(Equal(args.ExpectedName))
			Expect(eventLog.EventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.EventLog.Uid).To(Equal("0000-0000-0000"))
			Expect(eventLog.EventLog.RequestSubject).To(Equal("test-user"))
			Expect(eventLog.EventLog.OperationResult).To(Equal("allow"))

			Expect(eventLog.EventLog.ActionType).To(Equal(args.ExpectedActionType))
			Expect(eventLog.EventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.EventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.EventLog.FirmwareVersion).To(Equal("unknown"))

			if !args.ShouldLostNode {
				Expect(eventLog.EventLog.NodeNetworkAddress).To(Equal("127.0.0.1"))
			} else {
				Expect(eventLog.EventLog.NodeNetworkAddress).To(Equal("unknown"))
			}

			if !args.ShouldLostVD {
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
		Entry("VM Manage event should failed match if objectRef is nil", vmManageTestArgs{
			CustomObjectRefNil: true,
			ShouldFailMatch:    true,
		}),
		Entry("VM Manage event should failed match if stage is not ResponseComplete", vmManageTestArgs{
			CustomStage:     audit.StageRequestReceived,
			ShouldFailMatch: true,
		}),
		Entry("VM Manage event should failed match if resource is not virtualmachines", vmManageTestArgs{
			CustomObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations", Namespace: "test", Name: "test-vm"},
			ShouldFailMatch: true,
		}),
		Entry("VM Manage event chouldn't match if URI is not correct", vmManageTestArgs{
			EventURI:        "\x06/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			ShouldFailMatch: true,
		}),
		Entry("VM update event should filled without errors", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			EventVerb:          "update",
			ExpectedLevel:      "info",
			ExpectedName:       "VM update",
			ExpectedActionType: "update",
		}),
		Entry("VM patch event should filled without errors", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			EventVerb:          "patch",
			ExpectedLevel:      "info",
			ExpectedName:       "VM update",
			ExpectedActionType: "patch",
		}),
		Entry("VM delete event should filled without errors", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			EventVerb:          "delete",
			ExpectedLevel:      "warn",
			ExpectedName:       "VM deletion",
			ExpectedActionType: "delete",
		}),
		Entry("VM create event should filled without errors", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines",
			EventVerb:          "create",
			ExpectedLevel:      "info",
			ExpectedName:       "VM creation",
			ExpectedActionType: "create",
		}),
		Entry("VM manage event should filled without VM exist error", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines",
			EventVerb:          "create",
			ExpectedLevel:      "info",
			ExpectedName:       "VM creation",
			ExpectedActionType: "create",
			ShouldLostVM:       true,
		}),
		Entry("VM manage event should filled without VD exist error", vmManageTestArgs{
			EventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/dev/virtualmachines",
			EventVerb:          "create",
			ExpectedLevel:      "info",
			ExpectedName:       "VM creation",
			ExpectedActionType: "create",
			ShouldLostVD:       true,
			ShouldLostNode:     true,
		}),
	)
})
