/*
Copyright 2024 Flant JSC

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

package e2e

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/deckhouse/virtualization/test/e2e/blockdevice"
	"github.com/deckhouse/virtualization/test/e2e/controller"
	"github.com/deckhouse/virtualization/test/e2e/legacy"
	_ "github.com/deckhouse/virtualization/test/e2e/vm"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tests")
}

var _ = SynchronizedBeforeSuite(func() {
	controller.NewBeforeProcess1Body()
	legacy.NewBeforeProcess1Body()
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	controller.NewAfterAllProcessBody()
	legacy.NewAfterAllProcessBody()
})
