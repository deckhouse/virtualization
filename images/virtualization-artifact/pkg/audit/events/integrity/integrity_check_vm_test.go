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

package integrity_test

import (
	"os"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/audit"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events/integrity"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

type integrityCheckVMTestArgs struct {
	eventVerb           string
	expectedName        string
	expectedLevel       string
	shouldLostVMI       bool
	customObjectRef     *audit.ObjectReference
	customObjectRefNil  bool
	customStage         audit.Stage
	shouldFailMatch     bool
	shouldChecksumMatch bool
}

var _ = Describe("Integrity Check VM Events", func() {
	var event *audit.Event
	var vmi *virtv1.VirtualMachineInstance

	currentTime := time.Now()

	BeforeEach(func() {
		event = &audit.Event{
			TypeMeta:                 metav1.TypeMeta{},
			Level:                    audit.LevelRequestResponse,
			AuditID:                  "0000-0000-0000",
			Stage:                    audit.StageResponseComplete,
			Verb:                     "patch",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "internalvirtualizationvirtualmachineinstances",
				Name:      "test-vmi",
				Namespace: "test",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		vmi = &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test",
				UID:       "0000-0000-4567",
				Annotations: map[string]string{
					annotations.AnnIntegrityCoreChecksum:        "abc123",
					annotations.AnnIntegrityCoreChecksumApplied: "xyz789",
				},
			},
		}
	})

	DescribeTable("Checking Integrity Check VM events",
		func(args integrityCheckVMTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			informerList := &events.InformerListMock{
				GetInternalVMIInformerFunc: func() events.Indexer {
					return &events.IndexerMock{
						GetByKeyFunc: func(s string) (any, bool, error) {
							if args.shouldChecksumMatch {
								vmi.Annotations[annotations.AnnIntegrityCoreChecksumApplied] = vmi.Annotations[annotations.AnnIntegrityCoreChecksum]
							}

							unstruct, err := util.TypedObjectUnstructured(vmi)
							Expect(err).To(BeNil())

							return unstruct, !args.shouldLostVMI, nil
						},
					}
				},
			}

			eventLog := integrity.IntegrityCheckVM{
				Event:        event,
				TTLCache:     ttlCache,
				InformerList: informerList,
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

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			if args.shouldLostVMI {
				Expect(eventLog.Fill()).NotTo(BeNil())
				return
			}

			Expect(eventLog.Fill()).To(BeNil())

			Expect(eventLog.EventLog.Type).To(Equal("Integrity check"))
			Expect(eventLog.EventLog.Level).To(Equal("critical"))
			Expect(eventLog.EventLog.Name).To(Equal("VM config integrity check failed"))
			Expect(eventLog.EventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.EventLog.Uid).To(Equal("0000-0000-0000"))
			Expect(eventLog.EventLog.OperationResult).To(Equal("allow"))
			Expect(eventLog.EventLog.RequestSubject).To(Equal("test-user"))

			if args.shouldChecksumMatch {
				Expect(eventLog.ShouldLog()).To(BeFalse())
				return
			}

			Expect(eventLog.ShouldLog()).To(BeTrue())

			Expect(eventLog.EventLog.ObjectType).To(Equal("Virtual machine configuration"))
			Expect(eventLog.EventLog.VirtualMachineName).To(Equal("test-vm"))
			Expect(eventLog.EventLog.ControlMethod).To(Equal("Integrity Check"))
			Expect(eventLog.EventLog.ReactionType).To(Equal("info"))
			Expect(eventLog.EventLog.IntegrityCheckAlgo).To(Equal("sha256"))
			Expect(eventLog.EventLog.ReferenceChecksum).To(Equal("abc123"))
			Expect(eventLog.EventLog.CurrentChecksum).To(Equal("xyz789"))

			// Temporary redirect stdout to /dev/null
			defer func(stdout *os.File) {
				os.Stdout = stdout
			}(os.Stdout)
			os.Stdout = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

			Expect(eventLog.Log()).To(BeNil())
		},
		Entry("Integrity Check VM event should failed match if objectRef is nil", integrityCheckVMTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("Integrity Check VM event should failed match if stage is not ResponseComplete", integrityCheckVMTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("Integrity Check VM event should failed match if resource is not internalvirtualizationvirtualmachineinstances", integrityCheckVMTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachines", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("Integrity Check VM event should failed match if verb is not patch or update", integrityCheckVMTestArgs{
			eventVerb:       "get",
			shouldFailMatch: true,
		}),
		Entry("Integrity Check VM event should not log if checksums match", integrityCheckVMTestArgs{
			shouldChecksumMatch: true,
		}),
		Entry("Integrity Check VM event should log if checksums don't match", integrityCheckVMTestArgs{}),
		Entry("Integrity Check VM event should handle missing VMI", integrityCheckVMTestArgs{shouldLostVMI: true}),
	)
})
