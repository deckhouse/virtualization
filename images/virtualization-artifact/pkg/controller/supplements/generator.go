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

package supplements

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/strings"
)

const (
	tplCommon            = "d8v-%s-%s-%s"
	tplDVCRAuthSecret    = "d8v-%s-dvcr-auth-%s-%s"
	tplDVCRCABundle      = "d8v-%s-dvcr-ca-%s-%s"
	tplCABundle          = "d8v-%s-ca-%s-%s"
	tplImagePullSecret   = "d8v-%s-pull-image-%s-%s"
	tplImporterPod       = "d8v-%s-importer-%s-%s"
	tplBounderPod        = "d8v-%s-bounder-%s-%s"
	tplUploaderPod       = "d8v-%s-uploader-%s-%s"
	tplUploaderTLSSecret = "d8v-%s-tls-%s-%s"
)

// Generator calculates names for supplemental resources, e.g. ImporterPod, AuthSecret or CABundleConfigMap.
type Generator struct {
	LegacyGenerator
}

func NewGenerator(prefix, name, namespace string, uid types.UID) *Generator {
	return &Generator{
		LegacyGenerator: *NewLegacyGenerator(prefix, name, namespace, uid),
	}
}

func (g *Generator) generateName(template string, maxLength int) types.NamespacedName {
	maxNameLen := maxLength - len(template) + 6 - len(g.Prefix) - len(g.UID) // 6 is for %s placeholders
	name := fmt.Sprintf(template, g.Prefix, strings.ShortenString(g.Name, maxNameLen), g.UID)
	return types.NamespacedName{
		Name:      name,
		Namespace: g.Namespace,
	}
}

// DVCRAuthSecret returns name and namespace for auth Secret copy.
func (g *Generator) DVCRAuthSecret() types.NamespacedName {
	return g.generateName(tplDVCRAuthSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// DVCRAuthSecretForDV returns name and namespace for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *Generator) DVCRAuthSecretForDV() types.NamespacedName {
	return g.generateName(tplDVCRAuthSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// DVCRCABundleConfigMapForDV returns name and namespace for ConfigMap with ca.crt.
func (g *Generator) DVCRCABundleConfigMapForDV() types.NamespacedName {
	return g.generateName(tplDVCRCABundle, kvalidation.DNS1123SubdomainMaxLength)
}

// CABundleConfigMap returns name and namespace for ConfigMap which contains caBundle from dataSource.
func (g *Generator) CABundleConfigMap() types.NamespacedName {
	return g.generateName(tplCABundle, kvalidation.DNS1123SubdomainMaxLength)
}

// ImagePullSecret returns name and namespace for image pull secret for the containerImage dataSource.
func (g *Generator) ImagePullSecret() types.NamespacedName {
	return g.generateName(tplImagePullSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// ImporterPod generates name for importer Pod.
func (g *Generator) ImporterPod() types.NamespacedName {
	return g.generateName(tplImporterPod, kvalidation.DNS1123SubdomainMaxLength)
}

// BounderPod generates name for bounder Pod.
func (g *Generator) BounderPod() types.NamespacedName {
	return g.generateName(tplBounderPod, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderPod generates name for uploader Pod.
func (g *Generator) UploaderPod() types.NamespacedName {
	return g.generateName(tplUploaderPod, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderService generates name for uploader Service.
func (g *Generator) UploaderService() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123LabelMaxLength)
}

// UploaderIngress generates name for uploader Ingress.
func (g *Generator) UploaderIngress() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderTLSSecretForIngress generates name for uploader tls secret.
func (g *Generator) UploaderTLSSecretForIngress() types.NamespacedName {
	return g.generateName(tplUploaderTLSSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// DataVolume generates name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *Generator) DataVolume() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// NetworkPolicy generates name for NetworkPolicy.
func (g *Generator) NetworkPolicy() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}
