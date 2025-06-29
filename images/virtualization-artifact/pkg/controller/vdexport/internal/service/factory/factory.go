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
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

const (
	destinationAuthVol   = "dvcr-secret-vol"
	destinationCACertVol = "dvcr-ca-vol"
	exporterPortName     = "exporter"
	exporterPort         = 8444
)

type Factory interface {
	Pod() *corev1.Pod
	Service() *corev1.Service
	Ingress() *netv1.Ingress
}

type defaultFactory struct {
	sup    *supplements.Generator
	parent client.Object

	controllerName string
	image          string
	exportImage    string

	host string

	priorityClassName    string
	pullPolicy           corev1.PullPolicy
	imagePullSecrets     []corev1.LocalObjectReference
	resourceRequirements *corev1.ResourceRequirements
	extraLabels          map[string]string
	tolerations          []corev1.Toleration
	withCA               bool

	className     *string
	tlsSecretName *string
}

func (d defaultFactory) makeOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(d.parent, d.parent.GetObjectKind().GroupVersionKind())
}

func (d defaultFactory) podSelector() map[string]string {
	return map[string]string{
		annotations.LabelVirtualDataExportUID: string(d.parent.GetUID()),
	}
}
