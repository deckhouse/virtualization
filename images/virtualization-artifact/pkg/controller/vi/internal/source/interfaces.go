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

package source

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source/step"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . Importer Uploader Stat Bounder Handler Disk

type Importer interface {
	step.CreatePodStepImporter
	step.ReadyContainerRegistryStepImporter
	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
	GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error)
	Protect(ctx context.Context, pod *corev1.Pod, sup supplements.Generator) error
	Unprotect(ctx context.Context, pod *corev1.Pod, sup supplements.Generator) error
	Start(ctx context.Context, settings *importer.Settings, obj client.Object, sup supplements.Generator, caBundle *datasource.CABundle, opts ...service.Option) error
}

type Uploader interface {
	Start(ctx context.Context, settings *uploader.Settings, obj client.Object, sup supplements.Generator, caBundle *datasource.CABundle, opts ...service.Option) error
	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
	GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error)
	GetIngress(ctx context.Context, sup supplements.Generator) (*netv1.Ingress, error)
	GetService(ctx context.Context, sup supplements.Generator) (*corev1.Service, error)
	Protect(ctx context.Context, sup supplements.Generator, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error
	Unprotect(ctx context.Context, sup supplements.Generator, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error
	GetExternalURL(ctx context.Context, ing *netv1.Ingress) string
	GetInClusterURL(ctx context.Context, svc *corev1.Service) string
}

type Stat interface {
	step.CreatePodStepStat
	step.WaitForPodStepStat
	step.ReadyContainerRegistryStepStat
	IsUploadStarted(ownerUID types.UID, pod *corev1.Pod) bool
	IsUploaderReady(pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) bool
	GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *v1alpha2.StatusSpeed
}

type Bounder interface {
	step.CreateBounderPodStepBounder
	CleanUp(ctx context.Context, sup supplements.Generator) (bool, error)
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
}

type Disk interface {
	GetPersistentVolumeClaim(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error)
	CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error)
}
