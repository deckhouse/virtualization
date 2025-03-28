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
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	vmevent "github.com/deckhouse/virtualization-controller/pkg/audit/events/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	v1alpha "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type vmopTestArgs struct {
	VMOPType               v1alpha.VMOPType
	ExpectedName           string
	ExpectedLevel          string
	ExpectedActionType     string
	ShouldLostVMOP         bool
	ShouldLostVM           bool
	ShouldLostVD           bool
	ShouldLostNode         bool
	ShouldCorruptVMOPBytes bool
	CustomObjectRef        *audit.ObjectReference
	CustomObjectRefNil     bool
	CustomLevel            audit.Level
	ShouldFailMatch        bool
}

var _ = Describe("VMOP Events", func() {
	var event *audit.Event
	var vmop *v1alpha.VirtualMachineOperation
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
				Resource:  "virtualmachineoperations",
				Namespace: "test",
				Name:      "test-vmop",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		vmop = &v1alpha.VirtualMachineOperation{
			Spec: v1alpha.VirtualMachineOperationSpec{
				VirtualMachine: "test-vm",
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
		func(args vmopTestArgs) {
			bytes, _ := json.Marshal(vmop)
			event.ResponseObject = &runtime.Unknown{Raw: bytes}

			if args.ShouldCorruptVMOPBytes {
				bytes[0] = ^bytes[0]
			}

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
				GetVMOPInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return vmop, !args.ShouldLostVMOP, nil
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

			eventLog := vmevent.VMOPControl{
				Event:        event,
				InformerList: informerList,
				TTLCache:     ttlCache,
			}

			vmop.Spec.Type = args.VMOPType

			if args.CustomObjectRef != nil {
				event.ObjectRef = args.CustomObjectRef
			}

			if args.CustomObjectRefNil {
				event.ObjectRef = nil
			}

			if args.CustomLevel != "" {
				event.Level = args.CustomLevel
			}

			if args.ShouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.ShouldLostVMOP || args.ShouldLostVM || args.ShouldCorruptVMOPBytes {
				err := eventLog.Fill()
				Expect(err).NotTo(BeNil())
				return
			}

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.EventLog.Type).To(Equal("Control VM"))
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
		Entry("VMOP event should failed match if objectRef is nil", vmopTestArgs{
			CustomObjectRefNil: true,
			ShouldFailMatch: true,
		}),
		Entry("VMOP event should failed match if level is not RequestResponse", vmopTestArgs{
			CustomLevel:     audit.LevelMetadata,
			ShouldFailMatch: true,
		}),
		Entry("VMOP event should failed match if resource is not virtualmachineoperations", vmopTestArgs{
			CustomObjectRef: &audit.ObjectReference{Resource: "virtualmachines", Namespace: "test", Name: "test-vmop"},
			ShouldFailMatch: true,
		}),
		Entry("Start VMOP event should filled without errors", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStart,
			ExpectedName:       "VM started",
			ExpectedLevel:      "info",
			ExpectedActionType: "start",
		}),
		Entry("Stop VMOP event should filled without errors", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStop,
			ExpectedName:       "VM stopped",
			ExpectedLevel:      "warn",
			ExpectedActionType: "stop",
		}),
		Entry("Restart VMOP event should filled without errors", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeRestart,
			ExpectedName:       "VM restarted",
			ExpectedLevel:      "warn",
			ExpectedActionType: "restart",
		}),
		Entry("Migrate VMOP event should filled without errors", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeMigrate,
			ExpectedName:       "VM migrated",
			ExpectedLevel:      "warn",
			ExpectedActionType: "migrate",
		}),
		Entry("Evict VMOP event should filled without errors", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeEvict,
			ExpectedName:       "VM evicted",
			ExpectedLevel:      "warn",
			ExpectedActionType: "evict",
		}),
		Entry("Evict VMOP event should filled without errors, but with unknown VDs", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStart,
			ExpectedName:       "VM started",
			ExpectedLevel:      "info",
			ExpectedActionType: "start",
			ShouldLostVD:       true,
		}),
		Entry("Evict VMOP event should filled without errors, but with unknown Node's IPs", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStart,
			ExpectedName:       "VM started",
			ExpectedLevel:      "info",
			ExpectedActionType: "start",
			ShouldLostNode:     true,
		}),
		Entry("VMOP event should filled with VM exist error", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStart,
			ExpectedName:       "VM started",
			ExpectedLevel:      "info",
			ExpectedActionType: "start",
			ShouldLostVM:       true,
		}),
		Entry("VMOP event should filled with VMOP exist error", vmopTestArgs{
			VMOPType:           v1alpha.VMOPTypeStart,
			ExpectedName:       "VM started",
			ExpectedLevel:      "info",
			ExpectedActionType: "start",
			ShouldLostVMOP:     true,
		}),
		Entry("VMOP event should filled with JSON encode error", vmopTestArgs{
			VMOPType:               v1alpha.VMOPTypeStart,
			ExpectedName:           "VM started",
			ExpectedLevel:          "info",
			ExpectedActionType:     "start",
			ShouldCorruptVMOPBytes: true,
		}),
	)
})
