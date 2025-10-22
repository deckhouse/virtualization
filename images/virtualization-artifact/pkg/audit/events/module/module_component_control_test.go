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

package module

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

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/module"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type moduleComponentControlTestArgs struct {
	eventVerb          string
	expectedName       string
	expectedLevel      string
	expectedActionType string
	shouldLostModule   bool
	shouldLostPod      bool
	shouldLostNode     bool
	customUser         string
	customObjectRef    *audit.ObjectReference
	customObjectRefNil bool
	customStage        audit.Stage
	shouldFailMatch    bool
}

var _ = Describe("Module component control events", func() {
	var event *audit.Event
	var mod *module.Module
	var node *corev1.Node
	var pod *corev1.Pod

	currentTime := time.Now()

	BeforeEach(func() {
		event = &audit.Event{
			TypeMeta:                 metav1.TypeMeta{},
			Level:                    audit.LevelRequestResponse,
			AuditID:                  "0000-0000-0000",
			Stage:                    audit.StageResponseComplete,
			Verb:                     "update",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "pods",
				Name:      "virt-handler",
				Namespace: "d8-virtualization",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		mod = &module.Module{
			ObjectMeta: metav1.ObjectMeta{Name: "test-module", Namespace: "test", UID: "0000-0000-9876"},
			Properties: module.ModuleProperties{
				Version: "test-version",
			},
		}

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-handler",
				Namespace: "d8-virtualization",
				UID:       "0000-0000-4567",
				Annotations: map[string]string{
					annotations.AnnQemuVersion:    "9.9.9",
					annotations.AnnLibvirtVersion: "1.1.1",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node",
			},
		}

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node", UID: "0000-0000-4567"},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "127.0.0.1"}},
			},
		}
	})

	DescribeTable("Checking Module events",
		func(args moduleComponentControlTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetModuleInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							unstruct, err := util.TypedObjectUnstructured(mod)
							Expect(err).To(BeNil())

							return unstruct, !args.shouldLostModule, nil
						},
					}
				},
				GetPodInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							return pod, !args.shouldLostPod, nil
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

			eventLog := NewModuleComponentControl(&eventLoggerOptions)

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

			if args.customUser != "" {
				event.User.Username = args.customUser
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			Expect(eventLog.Fill()).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Virtualization control"))
			Expect(eventLog.eventLog.Level).To(Equal(args.expectedLevel))
			Expect(eventLog.eventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.OperationResult).To(Equal("allow"))
			Expect(eventLog.eventLog.ActionType).To(Equal(args.expectedActionType))
			Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))

			if args.shouldLostPod {
				Expect(eventLog.eventLog.QemuVersion).To(Equal("unknown"))
				Expect(eventLog.eventLog.LibvirtVersion).To(Equal("unknown"))
			} else {

				Expect(eventLog.eventLog.QemuVersion).To(Equal("9.9.9"))
				Expect(eventLog.eventLog.LibvirtVersion).To(Equal("1.1.1"))
			}

			if args.shouldLostNode || args.shouldLostPod {
				Expect(eventLog.eventLog.NodeNetworkAddress).To(Equal("unknown"))
			} else {
				Expect(eventLog.eventLog.NodeNetworkAddress).To(Equal("127.0.0.1"))
			}

			if args.shouldLostPod || args.shouldLostModule {
				Expect(eventLog.eventLog.VirtualizationVersion).To(Equal("unknown"))
			} else {
				Expect(eventLog.eventLog.VirtualizationVersion).To(Equal("test-version"))
			}

			Expect(eventLog.ShouldLog()).To(BeTrue())

			// Temporary redirect stdout to /dev/null
			defer func(stdout *os.File) {
				os.Stdout = stdout
			}(os.Stdout)
			os.Stdout = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

			Expect(eventLog.Log()).To(BeNil())
		},
		Entry("Module Control event should failed match if objectRef is nil", moduleComponentControlTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("Module Control event should failed match if stage is not ResponseComplete", moduleComponentControlTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if resource is not pods in d8-virtualization", moduleComponentControlTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if resource is pods cvi-importer", moduleComponentControlTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "pods", Namespace: "d8-virtualization", Name: "cvi-importer"},
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if user is replicaset-controller", moduleComponentControlTestArgs{
			customUser:      "system:serviceaccount:kube-system:replicaset-controller",
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if user is daemon-set-controller", moduleComponentControlTestArgs{
			customUser:      "system:serviceaccount:kube-system:daemon-set-controller",
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if user is job-controller", moduleComponentControlTestArgs{
			customUser:      "system:serviceaccount:kube-system:job-controller",
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if verb is get", moduleComponentControlTestArgs{
			eventVerb:       "get",
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if verb is list", moduleComponentControlTestArgs{
			eventVerb:       "list",
			shouldFailMatch: true,
		}),
		Entry("Module Control creation event shouldn't failed fill", moduleComponentControlTestArgs{
			eventVerb:          "create",
			expectedName:       "Component 'virt-handler' has been created by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "create",
		}),
		Entry("Module Control creation event shouldn't failed fill", moduleComponentControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Component 'virt-handler' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
		}),
		Entry("Module Control creation event shouldn't failed fill with losted module", moduleComponentControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Component 'virt-handler' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
			shouldLostModule:   true,
		}),
		Entry("Module Control creation event shouldn't failed fill with losted node", moduleComponentControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Component 'virt-handler' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
			shouldLostNode:     true,
		}),
		Entry("Module Control creation event shouldn't failed fill with losted pod", moduleComponentControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Component 'virt-handler' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
			shouldLostPod:      true,
		}),
	)
})
