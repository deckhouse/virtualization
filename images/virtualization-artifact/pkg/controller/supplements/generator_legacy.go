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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/strings"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LegacyGenerator generates names in the old format for backward compatibility
type LegacyGenerator struct {
	Prefix    string
	Name      string
	Namespace string
	UID       types.UID
}

func NewLegacyGenerator(prefix, name, namespace string, uid types.UID) *LegacyGenerator {
	return &LegacyGenerator{
		Prefix:    prefix,
		Name:      name,
		Namespace: namespace,
		UID:       uid,
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

func (g *LegacyGenerator) shortenNamespaced(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      strings.ShortenString(name, kvalidation.DNS1123SubdomainMaxLength),
		Namespace: g.Namespace,
	}
}
