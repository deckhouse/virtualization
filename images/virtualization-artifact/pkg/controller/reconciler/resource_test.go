/*
Copyright 2026 Flant JSC

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

package reconciler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
)

var _ = Describe("jsonPatchOpsForMetadataMap", func() {
	const path = "/metadata/labels"

	DescribeTable("chooses the mutation op based on the current map",
		func(current, changed map[string]string, expectedOp string) {
			ops := jsonPatchOpsForMetadataMap(path, current, changed)

			Expect(ops).To(HaveLen(2))

			Expect(ops[0].Op).To(Equal(patch.PatchTestOp))
			Expect(ops[0].Path).To(Equal(path))

			Expect(ops[1].Op).To(Equal(expectedOp))
			Expect(ops[1].Path).To(Equal(path))
			Expect(ops[1].Value).To(Equal(changed))
		},
		// Object has no labels at all: replace would be rejected by the API server,
		// so the patch must use add to create the field.
		Entry("nil current map uses add", nil, map[string]string{"a": "b"}, patch.PatchAddOp),
		Entry("empty current map uses add", map[string]string{}, map[string]string{"a": "b"}, patch.PatchAddOp),
		// Object already has labels: replace updates the existing field.
		Entry("non-empty current map uses replace", map[string]string{"x": "y"}, map[string]string{"a": "b"}, patch.PatchReplaceOp),
	)
})
