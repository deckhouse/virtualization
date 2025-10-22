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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/module"
	"github.com/deckhouse/virtualization-controller/pkg/audit/util"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

type moduleControlTestArgs struct {
	eventVerb              string
	expectedName           string
	expectedLevel          string
	expectedActionType     string
	shouldLostModule       bool
	shouldLostModuleConfig bool
	customObjectRef        *audit.ObjectReference
	customObjectRefNil     bool
	customStage            audit.Stage
	customDisabledModule   bool
	customNilEnabledModule bool
	shouldFailMatch        bool
}

var _ = Describe("Module control Events", func() {
	var event *audit.Event
	var modConfig *mcapi.ModuleConfig
	var mod *module.Module

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
				Resource: "moduleconfigs",
				Name:     "test-moduleconfig",
			},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "allow",
			},
		}

		modConfig = &mcapi.ModuleConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test-moduleconfig", Namespace: "test", UID: "0000-0000-4567"},
			Spec: mcapi.ModuleConfigSpec{
				Enabled: nil,
			},
		}

		mod = &module.Module{
			ObjectMeta: metav1.ObjectMeta{Name: "test-module", Namespace: "test", UID: "0000-0000-9876"},
			Properties: module.ModuleProperties{
				Version: "test-version",
			},
		}
	})

	DescribeTable("Checking Module events",
		func(args moduleControlTestArgs) {
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
				GetModuleConfigInformerFunc: func() cache.Store {
					return &cache.FakeCustomStore{
						GetByKeyFunc: func(s string) (any, bool, error) {
							unstruct, err := util.TypedObjectUnstructured(modConfig)
							Expect(err).To(BeNil())

							return unstruct, !args.shouldLostModuleConfig, nil
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
			}

			eventLog := NewModuleControl(&eventLoggerOptions)

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

			if args.customDisabledModule {
				modConfig.Spec.Enabled = ptr.To(false)
			}

			if args.shouldFailMatch {
				Expect(eventLog.IsMatched()).To(BeFalse())
				return
			}

			if args.customNilEnabledModule {
				modConfig.Spec.Enabled = nil
			}

			Expect(eventLog.IsMatched()).To(BeTrue())

			Expect(eventLog.Fill()).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Module control"))
			Expect(eventLog.eventLog.Level).To(Equal(args.expectedLevel))
			Expect(eventLog.eventLog.Name).To(Equal(args.expectedName))
			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.OperationResult).To(Equal("allow"))
			Expect(eventLog.eventLog.ActionType).To(Equal(args.expectedActionType))
			Expect(eventLog.eventLog.LibvirtVersion).To(Equal("unknown"))
			Expect(eventLog.eventLog.QemuVersion).To(Equal("unknown"))
			Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))
			Expect(eventLog.eventLog.NodeNetworkAddress).To(Equal("unknown"))

			if args.shouldLostModuleConfig || args.shouldLostModule {
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
		Entry("Module Control event should failed match if objectRef is nil", moduleControlTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("Module Control event should failed match if stage is not ResponseComplete", moduleControlTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if resource is not moduleconfigs", moduleControlTestArgs{
			customObjectRef: &audit.ObjectReference{Resource: "virtualmachineoperations", Namespace: "test", Name: "test-vm"},
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if verb is get", moduleControlTestArgs{
			eventVerb:       "get",
			shouldFailMatch: true,
		}),
		Entry("Module Control event should failed match if verb is list", moduleControlTestArgs{
			eventVerb:       "list",
			shouldFailMatch: true,
		}),
		Entry("Module Control creation event shouldn't failed fill", moduleControlTestArgs{
			eventVerb:          "create",
			expectedName:       "Module 'test-moduleconfig' has been created by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "create",
		}),
		Entry("Module Control update event shouldn't failed fill", moduleControlTestArgs{
			eventVerb:          "update",
			expectedName:       "Module 'test-moduleconfig' has been updated by 'test-user'",
			expectedLevel:      "info",
			expectedActionType: "update",
		}),
		Entry("Module Control deletion event shouldn't failed fill", moduleControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Module 'test-moduleconfig' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
		}),
		Entry("Module Control update with disable event shouldn't failed fill", moduleControlTestArgs{
			eventVerb:            "update",
			expectedName:         "Module 'test-moduleconfig' has been disabled by 'test-user'",
			expectedLevel:        "warn",
			expectedActionType:   "update",
			customDisabledModule: true,
		}),
		Entry("Module Control event shouldn't failed fill with losted moduleconfig", moduleControlTestArgs{
			eventVerb:              "delete",
			expectedName:           "Module 'test-moduleconfig' has been deleted by 'test-user'",
			expectedLevel:          "warn",
			expectedActionType:     "delete",
			shouldLostModuleConfig: true,
		}),
		Entry("Module Control event shouldn't failed fill with losted module", moduleControlTestArgs{
			eventVerb:          "delete",
			expectedName:       "Module 'test-moduleconfig' has been deleted by 'test-user'",
			expectedLevel:      "warn",
			expectedActionType: "delete",
			shouldLostModule:   true,
		}),
		Entry("Module Control event shouldn't failed fill with null enabled", moduleControlTestArgs{
			eventVerb:              "delete",
			expectedName:           "Module 'test-moduleconfig' has been deleted by 'test-user'",
			expectedLevel:          "warn",
			expectedActionType:     "delete",
			customNilEnabledModule: true,
		}),
	)
})
