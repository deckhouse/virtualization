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

type vmControlTestArgs struct {
	eventVerb                    string
	expectedName                 string
	expectedLevel                string
	expectedActionType           string
	shouldLostPod                bool
	shouldLostVM                 bool
	shouldLostVD                 bool
	shouldLostNode               bool
	shoulntLog                   bool
	customObjectRef              *audit.ObjectReference
	customObjectRefNil           bool
	customContainerStatusMessage string
	customEventUser              string
	customStage                  audit.Stage
	shouldFailMatch              bool
}

var _ = Describe("VMOP Events", func() {
	var event *audit.Event
	var vm *v1alpha2.VirtualMachine
	var vd *v1alpha2.VirtualDisk
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
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-test-vm",
				Namespace: "test",
				UID:       "0000-0000-4567",
				Labels: map[string]string{
					"vm.kubevirt.internal.virtualization.deckhouse.io/name": "test-vm",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "d8v-compute",
						Image: "test-image",
					},
				},
				NodeName: "test-node",
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:  "d8v-compute",
						State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Message: "guest-shutdown"}},
					},
				},
				Phase: corev1.PodRunning,
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
		func(args vmControlTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetPodInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return pod, !args.shouldLostPod, nil
						},
					}
				},
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

			eventLog := NewVMControl(&eventLoggerOptions)

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

			if args.shoulntLog {
				Expect(eventLog.ShouldLog()).To(BeFalse())
				return
			}

			Expect(eventLog.eventLog.Type).To(Equal("Control VM"))
			Expect(eventLog.eventLog.Level).To(Equal(args.expectedLevel))
			Expect(eventLog.eventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.OperationResult).To(Equal("allow"))
			Expect(eventLog.eventLog.ActionType).To(Equal(args.expectedActionType))

			if args.customEventUser == "some-user" ||
				(args.customContainerStatusMessage != "guest-shutdown" &&
					args.customContainerStatusMessage != "guest-reset") {
				Expect(eventLog.Fill()).To(BeNil())
				return
			}

			Expect(eventLog.eventLog.VirtualmachineUID).To(Equal("0000-0000-4567"))
			Expect(eventLog.eventLog.VirtualmachineOS).To(Equal("test-os"))
			Expect(eventLog.eventLog.QemuVersion).To(Equal("9.9.9"))
			Expect(eventLog.eventLog.LibvirtVersion).To(Equal("1.1.1"))

			if args.customEventUser != "" {
				Expect(eventLog.eventLog.RequestSubject).To(Equal(args.customEventUser))
			} else {
				Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))
			}

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
			expectedName:       "Virtual machine 'test-vm' has been killed abnormal way by 'test-user'",
			expectedActionType: "delete",
		}),
		Entry("VM stopped from OS by controller event should filled without errors", vmControlTestArgs{
			customEventUser:              "system:serviceaccount:d8-virtualization",
			customContainerStatusMessage: "guest-shutdown",
			expectedLevel:                "warn",
			expectedName:                 "Virtual machine 'test-vm' has been stopped from OS",
			expectedActionType:           "delete",
		}),
		Entry("VM restarted from OS by controller event should filled without errors", vmControlTestArgs{
			customEventUser:              "system:serviceaccount:d8-virtualization",
			customContainerStatusMessage: "guest-reset",
			expectedLevel:                "warn",
			expectedName:                 "Virtual machine 'test-vm' has been restarted from OS",
			expectedActionType:           "delete",
		}),
		Entry("VM deleted by node event should filled without errors", vmControlTestArgs{
			customEventUser: "system:node",
			shoulntLog:      true,
		}),
		Entry("VM deleted by controller with unknown termination message event should filled without errors", vmControlTestArgs{
			customContainerStatusMessage: "poped",
			customEventUser:              "system:serviceaccount:d8-virtualization",
			shoulntLog:                   true,
		}),
		Entry("VM deleted by user event with losted VM should filled without errors", vmControlTestArgs{
			expectedLevel:      "critical",
			expectedName:       "Virtual machine 'test-vm' has been killed abnormal way by 'test-user'",
			expectedActionType: "delete",
			shouldLostVM:       true,
		}),
		Entry("VM deleted by user event with losted VD and Node should filled without errors", vmControlTestArgs{
			expectedLevel:      "critical",
			expectedName:       "Virtual machine 'test-vm' has been killed abnormal way by 'test-user'",
			expectedActionType: "delete",
			shouldLostNode:     true,
			shouldLostVD:       true,
		}),
	)
})
