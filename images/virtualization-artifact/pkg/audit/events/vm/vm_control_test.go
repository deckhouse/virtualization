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

type vmControlTestArgs struct {
	eventVerb                    string
	expectedName                 string
	expectedLevel                string
	expectedActionType           string
	shouldLostPod                bool
	shouldLostVM                 bool
	shouldLostVD                 bool
	shouldLostNode               bool
	customObjectRef              *audit.ObjectReference
	customObjectRefNil           bool
	customContainerStatusMessage string
	customEventUser              string
	customStage                  audit.Stage
	shouldFailMatch              bool
}

var _ = Describe("VMOP Events", func() {
	var event *audit.Event
	var vm *v1alpha.VirtualMachine
	var vd *v1alpha.VirtualDisk
	var node *corev1.Node
	var pod *corev1.Pod

	currentTime := time.Now()

	BeforeEach(func() {
		event = &audit.Event{
			TypeMeta:                 metav1.TypeMeta{},
			Level:                    audit.LevelRequestResponse,
			AuditID:                  "0000-0000-0000",
			Stage:                    audit.StageResponseComplete,
			RequestURI:               "/",
			Verb:                     "delete",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "pods",
				Namespace: "test",
				Name:      "virt-launcher-test-vm",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "virt-launcher-test-vm", Namespace: "test", UID: "0000-0000-4567"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "compute",
						Image: "test-image",
					},
				},
				NodeName: "test-node",
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:  "compute",
						State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: "guest-shutdown"}},
					},
				},
				Phase: corev1.PodRunning,
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
		func(args vmControlTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetPodInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return pod, !args.shouldLostPod, nil
						},
					}
				},
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

			eventLog := vmevent.VMControl{
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

			if args.customStage != "" {
				event.Stage = args.customStage
			}

			if args.customContainerStatusMessage != "" {
				pod.Status.ContainerStatuses[0].State.Terminated.Message = args.customContainerStatusMessage
			}

			if args.customEventUser != "" {
				event.User.Username = args.customEventUser
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.shouldLostPod {
				Expect(eventLog.Fill()).NotTo(BeNil())
				return
			}

			err := eventLog.Fill()
			Expect(err).To(BeNil())

			Expect(eventLog.EventLog.Type).To(Equal("Control VM"))
			Expect(eventLog.EventLog.Level).To(Equal(args.expectedLevel))
			Expect(eventLog.EventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.EventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.EventLog.Uid).To(Equal("0000-0000-0000"))
			Expect(eventLog.EventLog.OperationResult).To(Equal("allow"))
			Expect(eventLog.EventLog.ActionType).To(Equal(args.expectedActionType))

			if args.customEventUser == "some-user" ||
				(args.customContainerStatusMessage != "guest-shutdown" &&
					args.customContainerStatusMessage != "guest-reset") {
				Expect(eventLog.Fill()).To(BeNil())
				return
			}

			Expect(eventLog.EventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.EventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.EventLog.FirmwareVersion).To(Equal("unknown"))

			if args.customEventUser != "" {
				Expect(eventLog.EventLog.RequestSubject).To(Equal(args.customEventUser))
			} else {
				Expect(eventLog.EventLog.RequestSubject).To(Equal("test-user"))
			}

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
		Entry("VM Manage event should failed match if objectRef is nil", vmControlTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("VM Manage event should failed match if stage is not ResponseComplete", vmControlTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("VM Control event should failed match if resource is not pods", vmControlTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("VM Control event should failed match if pod is not virt-launcher", vmControlTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "pods", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("VM Control event should failed match if verb is not delete", vmControlTestArgs{
			eventVerb:       "create",
			shouldFailMatch: true,
		}),
		Entry("VM create event should filled with Pod lost error", vmControlTestArgs{
			shouldLostPod: true,
		}),
		Entry("VM deleted by user event should filled without errors", vmControlTestArgs{
			expectedLevel:      "critical",
			expectedName:       "VM killed abnormal way",
			expectedActionType: "delete",
		}),
		Entry("VM stopped from OS by controller event should filled without errors", vmControlTestArgs{
			customEventUser:              "system:serviceaccount:d8-virtualization",
			customContainerStatusMessage: "guest-shutdown",
			expectedLevel:                "warn",
			expectedName:                 "VM stoped from OS",
			expectedActionType:           "delete",
		}),
		Entry("VM restarted from OS by controller event should filled without errors", vmControlTestArgs{
			customEventUser:              "system:serviceaccount:d8-virtualization",
			customContainerStatusMessage: "guest-reset",
			expectedLevel:                "warn",
			expectedName:                 "VM restarted from OS",
			expectedActionType:           "delete",
		}),
		Entry("VM deleted by node event should filled without errors", vmControlTestArgs{
			customEventUser:    "system:node",
			expectedLevel:      "info",
			expectedName:       "VM stopped by system",
			expectedActionType: "delete",
		}),
		Entry("VM deleted by controller with unknown termination message event should filled without errors", vmControlTestArgs{
			customContainerStatusMessage: "poped",
			customEventUser:              "system:serviceaccount:d8-virtualization",
			expectedLevel:                "warn",
			expectedName:                 "VM stopped by system",
			expectedActionType:           "delete",
		}),
		Entry("VM deleted by user event with losted VM should filled without errors", vmControlTestArgs{
			expectedLevel:      "critical",
			expectedName:       "VM killed abnormal way",
			expectedActionType: "delete",
			shouldLostVM:       true,
		}),
		Entry("VM deleted by user event with losted VD and Node should filled without errors", vmControlTestArgs{
			expectedLevel:      "critical",
			expectedName:       "VM killed abnormal way",
			expectedActionType: "delete",
			shouldLostNode:     true,
			shouldLostVD:       true,
		}),
	)
})
