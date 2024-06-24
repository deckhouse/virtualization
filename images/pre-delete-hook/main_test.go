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

package main

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	resourceName = "resources"
	resourceKind = "Resource"
)

var resourcesJsons = []string{
	`{
	"apiVersion": "group.io/v1",
	"kind": "` + resourceKind + `",
	"metadata": {
		  "namespace":  "default",
			"name": "resource-to-delete"
	}
}`,
	`{
	"apiVersion": "group.io/v1",
	"kind": "` + resourceKind + `",
	"metadata": {
		  "namespace":  "default",
			"name": "resource-to-keep"
	}
}`,
}

var resourceGVR = schema.GroupVersionResource{
	Group:    "group.io",
	Version:  "v1",
	Resource: resourceName,
}

var resources = []Resource{
	{
		Name:      "resource-to-delete",
		Namespace: "default",
		GVR:       resourceGVR,
	},
	{
		Name:      "resource-already-deleted",
		Namespace: "default",
		GVR:       resourceGVR,
	},
}

var _ = Describe("Pre delete hook tests", func() {
	var (
		fake *dynamicfake.FakeDynamicClient
		p    PreDeleteHook
	)

	BeforeEach(func() {
		fake = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		p.dynamicClient = fake
		p.resources = resources
		p.WaitTimeOut = 5 * time.Second
	})

	Describe("Run", func() {
		Context("Only the required resource should be deleted", func() {
			It("should return no error", func() {
				var err error

				for _, resJson := range resourcesJsons {
					resUnstructured := &unstructured.Unstructured{}
					err = resUnstructured.UnmarshalJSON([]byte(resJson))
					Expect(err).NotTo(HaveOccurred())
					err = fake.Tracker().Add(resUnstructured)
					Expect(err).NotTo(HaveOccurred())
				}

				p.Run()

				_, err = fake.Tracker().Get(resourceGVR, "default", "resource-to-delete")
				Expect(err).Should(HaveOccurred())
				Expect(errors.IsNotFound(err)).Should(BeTrue())

				_, err = fake.Tracker().Get(resourceGVR, "default", "resource-to-keep")
				Expect(err).Should(Succeed())
			})
		})
	})
})

func TestDiscoverer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pre Delete Hook Test Suite")
}
