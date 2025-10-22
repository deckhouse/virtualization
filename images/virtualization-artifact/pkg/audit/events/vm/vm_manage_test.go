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
	v1alpha "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type vmManageTestArgs struct {
	eventURI           string
	eventVerb          string
	expectedName       string
	expectedLevel      string
	expectedActionType string
	shouldLostVM       bool
	shouldLostVD       bool
	shouldLostNode     bool
	customObjectRef    *audit.ObjectReference
	customObjectRefNil bool
	customStage        audit.Stage
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
				Versions: v1alpha.Versions{
					Qemu:    "9.9.9",
					Libvirt: "1.1.1",
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

			eventLog := NewVMManage(&eventLoggerOptions)

			if args.eventVerb != "" {
				event.Verb = args.eventVerb
			}

			if args.eventURI != "" {
				event.RequestURI = args.eventURI
			}

			if args.customObjectRef != nil {
				event.ObjectRef = args.customObjectRef
			}

			if args.customObjectRefNil {
				event.ObjectRef = nil
			}

			if args.customStage != "" {
				event.Stage = args.customStage
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.shouldLostVM {
				err := eventLog.Fill()
				Expect(err).To(BeNil())
				return
			}

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Manage VM"))
			Expect(eventLog.eventLog.Level).To(Equal(args.expectedLevel))
			Expect(eventLog.eventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))
			Expect(eventLog.eventLog.OperationResult).To(Equal("allow"))

			Expect(eventLog.eventLog.ActionType).To(Equal(args.expectedActionType))
			Expect(eventLog.eventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.eventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.eventLog.QemuVersion).To(Equal("9.9.9"))
			Expect(eventLog.eventLog.LibvirtVersion).To(Equal("1.1.1"))

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
		Entry("VM Manage event should failed match if objectRef is nil", vmManageTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("VM Manage event should failed match if stage is not ResponseComplete", vmManageTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("VM Manage event should failed match if resource is not virtualmachines", vmManageTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("VM Manage event chouldn't match if URI is not correct", vmManageTestArgs{
			eventURI:        "\x06/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			shouldFailMatch: true,
		}),
		Entry("VM update event should filled without errors", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			eventVerb:          "update",
			expectedLevel:      "info",
			expectedName:       "Virtual machine 'test-vm' has been updated by 'test-user'",
			expectedActionType: "update",
		}),
		Entry("VM patch event should filled without errors", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			eventVerb:          "patch",
			expectedLevel:      "info",
			expectedName:       "Virtual machine 'test-vm' has been updated by 'test-user'",
			expectedActionType: "patch",
		}),
		Entry("VM delete event should filled without errors", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines/test-vm",
			eventVerb:          "delete",
			expectedLevel:      "warn",
			expectedName:       "Virtual machine 'test-vm' has been deleted by 'test-user'",
			expectedActionType: "delete",
		}),
		Entry("VM create event should filled without errors", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines",
			eventVerb:          "create",
			expectedLevel:      "info",
			expectedName:       "Virtual machine 'test-vm' has been created by 'test-user'",
			expectedActionType: "create",
		}),
		Entry("VM manage event should filled without VM exist error", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines",
			eventVerb:          "create",
			expectedLevel:      "info",
			expectedName:       "Virtual machine 'test-vm' has been created by 'test-user'",
			expectedActionType: "create",
			shouldLostVM:       true,
		}),
		Entry("VM manage event should filled without VD exist error", vmManageTestArgs{
			eventURI:           "/apis/virtualization.deckhouse.io/v1alpha2/namespaces/test/virtualmachines",
			eventVerb:          "create",
			expectedLevel:      "info",
			expectedName:       "Virtual machine 'test-vm' has been created by 'test-user'",
			expectedActionType: "create",
			shouldLostVD:       true,
			shouldLostNode:     true,
		}),
	)
})
