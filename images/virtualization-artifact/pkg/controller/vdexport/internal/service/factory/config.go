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

package factory

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

type Config struct {
	// Common Required fields
	ControllerName string

	// Required fields for pod
	Image       string
	ExportImage string

	// Required fields for ingress
	Host string

	// Optional fields for pod
	PriorityClassName    string
	PullPolicy           corev1.PullPolicy
	ImagePullSecrets     []corev1.LocalObjectReference
	ResourceRequirements *corev1.ResourceRequirements
	ExtraLabels          map[string]string
	Tolerations          []corev1.Toleration
	WithCA               bool

	// Optional fields for ingress
	ClassName     *string
	TLSSecretName *string
}

func (c *Config) Validate() error {
	if c.ControllerName == "" {
		return fmt.Errorf("ControllerName is required")
	}

	if c.Image == "" {
		return fmt.Errorf("Image is required")
	}

	if c.Host == "" {
		return fmt.Errorf("Host is required")
	}

	return nil
}

func (c *Config) Complete(sup *supplements.Generator, parent client.Object) (Factory, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	f := &defaultFactory{
		sup:    sup,
		parent: parent,

		controllerName: c.ControllerName,
		image:          c.Image,
		exportImage:    c.ExportImage,

		host: c.Host,

		priorityClassName:    c.PriorityClassName,
		pullPolicy:           c.PullPolicy,
		imagePullSecrets:     c.ImagePullSecrets,
		resourceRequirements: c.ResourceRequirements,
		extraLabels:          c.ExtraLabels,
		tolerations:          c.Tolerations,
		withCA:               c.WithCA,

		className:     c.ClassName,
		tlsSecretName: c.TLSSecretName,
	}

	if f.pullPolicy == "" {
		f.pullPolicy = corev1.PullIfNotPresent
	}
	if f.resourceRequirements == nil {
		f.resourceRequirements = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(100, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(128*1024*1024, resource.DecimalSI),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewQuantity(200, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(256*1024*1024, resource.DecimalSI),
			},
		}
	}

	return f, nil
}
