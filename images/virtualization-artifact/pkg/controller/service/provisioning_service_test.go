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

package service

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProvisioningService", func() {
	var provisioningService *ProvisioningService

	BeforeEach(func() {
		provisioningService = NewProvisioningService(nil)
	})

	DescribeTable("ValidateUserDataLen",
		func(userData string, expectedErr error) {
			err := provisioningService.ValidateUserDataLen(userData)
			if expectedErr != nil {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(expectedErr))
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry(
			"empty userdata",
			"",
			ErrUserDataEmpty,
		),
		Entry(
			"userdata exceeds max limit",
			string(make([]byte, cloudInitUserMaxLen+1)),
			ErrUserDataTooLong,
		),
		Entry(
			"userdata is within limit",
			string(make([]byte, cloudInitUserMaxLen-1)),
			nil,
		),
	)
})
