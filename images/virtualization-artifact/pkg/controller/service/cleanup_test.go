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

var _ = Describe("Cleanup", func() {
	Describe("MergeCleanUpReasons", func() {
		It("skips empty reasons and keeps unique reasons in order", func() {
			Expect(MergeCleanUpReasons(
				"",
				"waiting for PersistentVolumeClaim deletion default/vd",
				"waiting for PersistentVolumeClaim deletion default/vd",
				"waiting for Pod deletion default/importer",
			)).To(Equal("waiting for PersistentVolumeClaim deletion default/vd; waiting for Pod deletion default/importer"))
		})
	})
})
