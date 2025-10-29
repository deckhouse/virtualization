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

package forbid

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	authrv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"

	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func TestModuleEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Module Events Test Suite")
}

type ForbidTestArgs struct {
	eventVerb              string
	shouldAllow            bool
	customObjectRef        *audit.ObjectReference
	customObjectRefNil     bool
	customStage            audit.Stage
	canUpdateModuleConfigs bool
	canUpdateVMClasses     bool
	shouldFailMatch        bool
	shouldClientFail       bool
}

var _ = Describe("Forbid Events", func() {
	var event *audit.Event

	currentTime := time.Now()

	BeforeEach(func() {
		event = &audit.Event{
			TypeMeta:                 metav1.TypeMeta{},
			Level:                    audit.LevelRequestResponse,
			AuditID:                  "0000-0000-0000",
			Stage:                    audit.StageResponseComplete,
			Verb:                     "create",
			User:                     authnv1.UserInfo{Username: "test-user", UID: "0000-0000-1234"},
			RequestReceivedTimestamp: metav1.MicroTime{Time: currentTime},
			ObjectRef: &audit.ObjectReference{
				Resource:  "pods",
				Name:      "test-vmi",
				Namespace: "test",
			},
			SourceIPs: []string{"127.0.0.1"},
			Annotations: map[string]string{
				annotations.AnnAuditDecision: "forbid",
				annotations.AnnAuditReason:   "some reason",
			},
		}
	})

	DescribeTable("Checking Forbid events",
		func(args ForbidTestArgs) {
			ttlCache := &events.TTLCacheMock{
				GetFunc: func(key string) (any, bool) {
					return nil, false
				},
			}

			fakeClient := fake.NewSimpleClientset()
			fakeClient.Fake.PrependReactor("create", "subjectaccessreviews", func(action kubetesting.Action) (
				handled bool,
				ret runtime.Object,
				err error,
			) {
				sa := action.(kubetesting.CreateAction).GetObject().(*authrv1.SubjectAccessReview)
				sa.Name = uuid.New().String()

				if sa.Spec.ResourceAttributes.Resource == "moduleconfigs" {
					sa.Status.Allowed = args.canUpdateModuleConfigs
				}

				if sa.Spec.ResourceAttributes.Resource == "virtualmachineclasses" {
					sa.Status.Allowed = args.canUpdateVMClasses
				}

				if args.shouldClientFail {
					return true, nil, fmt.Errorf("some error")
				}

				return false, sa, nil
			})

			eventLoggerOptions := events.EventLoggerOptionsMock{
				GetCtxFunc: context.Background,
				GetEventFunc: func() *audit.Event {
					return event
				},
				GetClientFunc: func() kubernetes.Interface {
					return fakeClient
				},
				GetTTLCacheFunc: func() events.TTLCache {
					return ttlCache
				},
			}

			eventLog := NewForbid(&eventLoggerOptions)

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

			if args.shouldAllow {
				event.Annotations[annotations.AnnAuditDecision] = "allow"
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

			Expect(eventLog.Fill()).To(BeNil())

			Expect(eventLog.eventLog.Type).To(Equal("Forbidden operation"))
			Expect(eventLog.eventLog.Level).To(Equal("warn"))

			Expect(eventLog.eventLog.Name).To(Equal("User 'test-user' attempted to perform a forbidden operation 'create' on resource 'pods/test/test-vmi'."))

			Expect(eventLog.eventLog.Datetime).To(Equal(currentTime.Format(time.RFC3339)))
			Expect(eventLog.eventLog.UID).To(Equal("0000-0000-0000"))
			Expect(eventLog.eventLog.OperationResult).To(Equal("forbid"))
			Expect(eventLog.eventLog.RequestSubject).To(Equal("test-user"))

			Expect(eventLog.eventLog.IsAdmin).To(Equal(args.canUpdateModuleConfigs || args.canUpdateVMClasses))
			Expect(eventLog.eventLog.SourceIP).To(Equal("127.0.0.1"))
			Expect(eventLog.eventLog.ForbidReason).To(Equal("some reason"))

			Expect(eventLog.ShouldLog()).To(BeTrue())

			// Temporary redirect stdout to /dev/null
			defer func(stdout *os.File) {
				os.Stdout = stdout
			}(os.Stdout)
			os.Stdout = os.NewFile(uintptr(syscall.Stdin), os.DevNull)

			Expect(eventLog.Log()).To(BeNil())
		},
		Entry("Forbid event should failed match if objectRef is nil", ForbidTestArgs{
			customObjectRefNil: true,
			shouldFailMatch:    true,
		}),
		Entry("Forbid event should failed match if stage is not ResponseComplete", ForbidTestArgs{
			customStage:     audit.StageRequestReceived,
			shouldFailMatch: true,
		}),
		Entry("Forbid event should failed match if ann audit decision is not forbid", ForbidTestArgs{
			shouldAllow:     true,
			shouldFailMatch: true,
		}),
		Entry("Forbid event shouldn't fail fill if user can update moduleconfigs", ForbidTestArgs{
			canUpdateModuleConfigs: true,
		}),
		Entry("Forbid event shouldn't fail fill if user can update virtualmachineclasses", ForbidTestArgs{
			canUpdateVMClasses: true,
		}),
		Entry("Forbid event shouldn't fail fill if user can't update moduleconfigs", ForbidTestArgs{
			canUpdateModuleConfigs: false,
		}),
		Entry("Forbid event shouldn't fail fill if user can't update virtualmachineclasses", ForbidTestArgs{
			canUpdateVMClasses: false,
		}),
		Entry("Forbid event shouldn't fail fill if client return error", ForbidTestArgs{
			shouldClientFail: true,
		}),
	)
})
