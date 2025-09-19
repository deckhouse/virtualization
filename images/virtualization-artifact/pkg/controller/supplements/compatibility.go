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

package supplements

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LegacyGenerator generates names in the old format for backward compatibility
type LegacyGenerator struct {
	*Generator
}

func NewLegacyGenerator(prefix, name, namespace string, uid types.UID) *LegacyGenerator {
	return &LegacyGenerator{
		Generator: NewGenerator(prefix, name, namespace, uid),
	}
}

// DVCRAuthSecret returns old format name for auth Secret copy.
func (g *LegacyGenerator) DVCRAuthSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DVCRAuthSecretForDV returns old format name for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *LegacyGenerator) DVCRAuthSecretForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-dv-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DVCRCABundleConfigMapForDV returns old format name for ConfigMap with ca.crt.
func (g *LegacyGenerator) DVCRCABundleConfigMapForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-ca-dv-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// CABundleConfigMap returns old format name for ConfigMap which contains caBundle from dataSource.
func (g *LegacyGenerator) CABundleConfigMap() types.NamespacedName {
	name := fmt.Sprintf("%s-ca-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// ImagePullSecret returns old format name for image pull secret for the containerImage dataSource.
func (g *LegacyGenerator) ImagePullSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-pull-image-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// ImporterPod generates old format name for importer Pod.
func (g *LegacyGenerator) ImporterPod() types.NamespacedName {
	name := fmt.Sprintf("%s-importer-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// BounderPod generates old format name for bounder Pod.
func (g *LegacyGenerator) BounderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-bounder-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// UploaderPod generates old format name for uploader Pod.
func (g *LegacyGenerator) UploaderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// UploaderService generates old format name for uploader Service.
func (g *LegacyGenerator) UploaderService() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-svc-%s", g.Prefix, string(g.UID))
	return g.shortenNamespaced(name)
}

// UploaderIngress generates old format name for uploader Ingress.
func (g *LegacyGenerator) UploaderIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-ingress-%s", g.Prefix, string(g.UID))
	return g.shortenNamespaced(name)
}

// UploaderTLSSecretForIngress generates old format name for uploader tls secret.
func (g *LegacyGenerator) UploaderTLSSecretForIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-tls-ing-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DataVolume generates old format name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *LegacyGenerator) DataVolume() types.NamespacedName {
	dvName := fmt.Sprintf("%s-%s-%s", g.Prefix, g.Name, string(g.UID))
	return g.shortenNamespaced(dvName)
}

func (g *LegacyGenerator) PersistentVolumeClaim() types.NamespacedName {
	return g.DataVolume()
}

// NetworkPolicy generates old format name for NetworkPolicy.
func (g *LegacyGenerator) NetworkPolicy() types.NamespacedName {
	// Old network policies used DataVolume/Pod names directly
	return g.DataVolume()
}

// FindResourceWithFallback attempts to find a resource with new naming,
// falling back to old naming if not found
func FindResourceWithFallback[T client.Object](ctx context.Context, c client.Client, newName, oldName types.NamespacedName, obj T) error {
	// Try new name first
	err := c.Get(ctx, newName, obj)
	if err == nil {
		return nil
	}

	if !k8serrors.IsNotFound(err) {
		return err
	}

	// Fallback to old name
	return c.Get(ctx, oldName, obj)
}

// GetPVCWithFallback attempts to find a PVC with new naming,
// falling back to old naming if not found
func GetPVCWithFallback(ctx context.Context, c client.Client, gen *Generator) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.PersistentVolumeClaim(), legacyGen.PersistentVolumeClaim(), pvc)
	return pvc, err
}

// FindImporterPodWithFallback attempts to find an importer pod with new naming,
// falling back to old naming if not found
func FindImporterPodWithFallback(ctx context.Context, c client.Client, gen *Generator) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.ImporterPod(), legacyGen.ImporterPod(), pod)
	return pod, err
}

// FindUploaderPodWithFallback attempts to find an uploader pod with new naming,
// falling back to old naming if not found
func FindUploaderPodWithFallback(ctx context.Context, c client.Client, gen *Generator) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.UploaderPod(), legacyGen.UploaderPod(), pod)
	return pod, err
}

// FindBounderPodWithFallback attempts to find a bounder pod with new naming,
// falling back to old naming if not found
func FindBounderPodWithFallback(ctx context.Context, c client.Client, gen *Generator) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.BounderPod(), legacyGen.BounderPod(), pod)
	return pod, err
}

// FindUploaderServiceWithFallback attempts to find an uploader service with new naming,
// falling back to old naming if not found
func FindUploaderServiceWithFallback(ctx context.Context, c client.Client, gen *Generator) (*corev1.Service, error) {
	svc := &corev1.Service{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.UploaderService(), legacyGen.UploaderService(), svc)
	return svc, err
}

// FindUploaderIngressWithFallback attempts to find an uploader ingress with new naming,
// falling back to old naming if not found
func FindUploaderIngressWithFallback(ctx context.Context, c client.Client, gen *Generator) (*netv1.Ingress, error) {
	ing := &netv1.Ingress{}
	legacyGen := NewLegacyGenerator(gen.Prefix, gen.Name, gen.Namespace, gen.UID)

	err := FindResourceWithFallback(ctx, c, gen.UploaderIngress(), legacyGen.UploaderIngress(), ing)
	return ing, err
}
