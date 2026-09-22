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

package watcher

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// recordingController keeps the sources a watcher registers; nothing is started.
type recordingController struct {
	controller.Controller
	sources []string
}

func (c *recordingController) Watch(src source.TypedSource[reconcile.Request]) error {
	c.sources = append(c.sources, fmt.Sprint(src))
	return nil
}

type nilCacheManager struct {
	manager.Manager
}

func (nilCacheManager) GetCache() cache.Cache { return nil }

var _ = Describe("live migration watchers", func() {
	It("watch their own kinds", func() {
		ctr := &recordingController{}
		Expect(NewKVVMIWatcher().Watch(nilCacheManager{}, ctr)).To(Succeed())
		Expect(NewKVVMIMWatcher().Watch(nilCacheManager{}, ctr)).To(Succeed())
		Expect(ctr.sources).To(HaveLen(2))
		Expect(ctr.sources[0]).To(Equal("kind source: *v1.VirtualMachineInstance"))
		Expect(ctr.sources[1]).To(Equal("kind source: *v1.VirtualMachineInstanceMigration"))
	})
})
