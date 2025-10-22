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
	"encoding/json"
	"os"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type vmopTestArgs struct {
	vmopType               v1alpha2.VMOPType
	expectedName           string
	expectedLevel          string
	expectedActionType     string
	shouldLostVMOP         bool
	shouldLostVM           bool
	shouldLostVD           bool
	shouldLostNode         bool
	shouldCorruptVMOPBytes bool
	customObjectRef        *audit.ObjectReference
	customObjectRefNil     bool
	customLevel            audit.Level
	shouldFailMatch        bool
}

var _ = Describe("VMOP Events", func() {
	var event *audit.Event
	var vmop *v1alpha2.VirtualMachineOperation
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
			RequestURI:               "/",
			Verb:                     "create",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "virtualmachineoperations",
				Namespace: "test",
				Name:      "test-vmop",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		vmop = &v1alpha2.VirtualMachineOperation{
			Spec: v1alpha2.VirtualMachineOperationSpec{
				VirtualMachine: "test-vm",
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
		func(args vmopTestArgs) {
			bytes, _ := json.Marshal(vmop)
			event.ResponseObject = &runtime.Unknown{Raw: bytes}

			if args.shouldCorruptVMOPBytes {
				bytes[0] = ^bytes[0]
			}

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
				GetVMOPInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vmop, !args.shouldLostVMOP, nil
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

			eventLog := NewVMOPControl(&eventLoggerOptions)

			vmop.Spec.Type = args.vmopType

			if args.customObjectRef != nil {
				event.ObjectRef = args.customObjectRef
			}

			if args.customObjectRefNil {
				event.ObjectRef = nil
			}

			if args.customLevel != "" {
				event.Level = args.customLevel
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.shouldLostVMOP || args.shouldLostVM || args.shouldCorruptVMOPBytes {
				err := eventLog.Fill()
				Expect(err).NotTo(BeNil())
				return
			}

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Control VM"))
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
		Entry("VMOP event should failed match if objectRef is nil", vmopTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("VMOP event should failed match if level is not RequestResponse", vmopTestArgs{
			customLevel:     audit.LevelMetadata,
			shouldFailMatch: true,
		}),
		Entry("VMOP event should failed match if resource is not virtualmachineoperations", vmopTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachines", Namespace: "test", Name: "test-vmop"},
			shouldFailMatch: true,
		}),
		Entry("Start VMOP event should filled without errors", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStart,
			expectedName:       "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "start",
		}),
		Entry("Stop VMOP event should filled without errors", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStop,
			expectedName:       "Virtual machine 'test-vm' has been stopped by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "stop",
		}),
		Entry("Restart VMOP event should filled without errors", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeRestart,
			expectedName:       "Virtual machine 'test-vm' has been restarted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "restart",
		}),
		Entry("Migrate VMOP event should filled without errors", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeMigrate,
			expectedName:       "Virtual machine 'test-vm' has been migrated by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "migrate",
		}),
		Entry("Evict VMOP event should filled without errors", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeEvict,
			expectedName:       "Virtual machine 'test-vm' has been evicted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "evict",
		}),
		Entry("Evict VMOP event should filled without errors, but with unknown VDs", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStart,
			expectedName:       "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "start",
			shouldLostVD:       true,
		}),
		Entry("Evict VMOP event should filled without errors, but with unknown Node's IPs", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStart,
			expectedName:       "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "start",
			shouldLostNode:     true,
		}),
		Entry("VMOP event should filled with VM exist error", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStart,
			expectedName:       "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "start",
			shouldLostVM:       true,
		}),
		Entry("VMOP event should filled with VMOP exist error", vmopTestArgs{
			vmopType:           v1alpha2.VMOPTypeStart,
			expectedName:       "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "start",
			shouldLostVMOP:     true,
		}),
		Entry("VMOP event should filled with JSON encode error", vmopTestArgs{
			vmopType:               v1alpha2.VMOPTypeStart,
			expectedName:           "Virtual machine 'test-vm' has been started by 'test-user'",
			expectedLevel:          "info",
			expectedActionType:     "start",
			shouldCorruptVMOPBytes: true,
		}),
	)
})
