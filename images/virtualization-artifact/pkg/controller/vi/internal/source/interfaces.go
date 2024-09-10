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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate moq -rm -out mock.go . Importer Uploader Stat

type Importer interface {
	Start(ctx context.Context, settings *importer.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error
	StartFromPVC(ctx context.Context, settings *importer.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle, pvcName string, pvcNamespace string) error
	CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error)
	CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error)
	GetPod(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error)
	Protect(ctx context.Context, pod *corev1.Pod) error
	Unprotect(ctx context.Context, pod *corev1.Pod) error
}

type Uploader interface {
	Start(ctx context.Context, settings *uploader.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error
	CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error)
	CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error)
	GetPod(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error)
	GetIngress(ctx context.Context, sup *supplements.Generator) (*netv1.Ingress, error)
	GetService(ctx context.Context, sup *supplements.Generator) (*corev1.Service, error)
	Protect(ctx context.Context, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error
	Unprotect(ctx context.Context, pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) error
}

type Stat interface {
	GetFormat(pod *corev1.Pod) string
	GetCDROM(pod *corev1.Pod) bool
	GetSize(pod *corev1.Pod) virtv2.ImageStatusSize
	GetDVCRImageName(pod *corev1.Pod) string
	GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *virtv2.StatusSpeed
	GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string
	IsUploadStarted(ownerUID types.UID, pod *corev1.Pod) bool
	CheckPod(pod *corev1.Pod) error
	GetAdjustImageSize(unpackedSizeBytes resource.Quantity) resource.Quantity
}
