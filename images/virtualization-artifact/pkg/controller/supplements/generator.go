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
	tplCommon              = "d8v-%s-%s-%s"
	tplDVCRAuthSecret      = "d8v-%s-dvcr-auth-%s-%s"
	tplDVCRAuthSecretForDV = "d8v-%s-dvcr-auth-dv-%s-%s"
	tplDVCRCABundle        = "d8v-%s-dvcr-ca-%s-%s"
	tplCABundle            = "d8v-%s-ca-%s-%s"
	tplImagePullSecret     = "d8v-%s-pull-image-%s-%s"
	tplImporterPod         = "d8v-%s-importer-%s-%s"
	tplBounderPod          = "d8v-%s-bounder-%s-%s"
	tplUploaderPod         = "d8v-%s-uploader-%s-%s"
	tplUploaderTLSSecret   = "d8v-%s-tls-%s-%s"
)

type Generator interface {
	Namespace() string
	Name() string
	UID() types.UID

	BounderPod() types.NamespacedName
	ImporterPod() types.NamespacedName
	UploaderPod() types.NamespacedName
	UploaderService() types.NamespacedName
	UploaderIngress() types.NamespacedName
	DataVolume() types.NamespacedName
	PersistentVolumeClaim() types.NamespacedName
	CABundleConfigMap() types.NamespacedName
	DVCRAuthSecret() types.NamespacedName
	DVCRCABundleConfigMapForDV() types.NamespacedName
	DVCRAuthSecretForDV() types.NamespacedName
	UploaderTLSSecretForIngress() types.NamespacedName
	ImagePullSecret() types.NamespacedName
	NetworkPolicy() types.NamespacedName
	CommonSupplement() types.NamespacedName

	LegacyBounderPod() types.NamespacedName
	LegacyImporterPod() types.NamespacedName
	LegacyUploaderPod() types.NamespacedName
	LegacyUploaderService() types.NamespacedName
	LegacyUploaderIngress() types.NamespacedName
	LegacyDataVolume() types.NamespacedName
	LegacyPersistentVolumeClaim() types.NamespacedName
	LegacyCABundleConfigMap() types.NamespacedName
	LegacyDVCRAuthSecret() types.NamespacedName
	LegacyDVCRCABundleConfigMapForDV() types.NamespacedName
	LegacyDVCRAuthSecretForDV() types.NamespacedName
	LegacyUploaderTLSSecretForIngress() types.NamespacedName
	LegacyImagePullSecret() types.NamespacedName
	LegacySnapshotSupplement() types.NamespacedName
}

// Generator calculates names for supplemental resources, e.g. ImporterPod, AuthSecret or CABundleConfigMap.
type generator struct {
	prefix    string
	name      string
	namespace string
	uid       types.UID
}

func NewGenerator(prefix, name, namespace string, uid types.UID) Generator {
	return &generator{
		prefix:    prefix,
		name:      name,
		namespace: namespace,
		uid:       uid,
	}
}

func (g *generator) generateName(template string, maxLength int) types.NamespacedName {
	maxNameLen := maxLength - len(template) + 6 - len(g.prefix) - len(g.uid) // 6 is for %s placeholders
	name := fmt.Sprintf(template, g.prefix, strings.ShortenString(g.name, maxNameLen), g.UID())
	return types.NamespacedName{
		Name:      name,
		Namespace: g.namespace,
	}
}

func (g *generator) Namespace() string {
	return g.namespace
}

func (g *generator) Name() string {
	return g.name
}

func (g *generator) UID() types.UID {
	return g.uid
}

// DVCRAuthSecret returns name and namespace for auth Secret copy.
func (g *generator) DVCRAuthSecret() types.NamespacedName {
	return g.generateName(tplDVCRAuthSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// DVCRAuthSecretForDV returns name and namespace for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *generator) DVCRAuthSecretForDV() types.NamespacedName {
	return g.generateName(tplDVCRAuthSecretForDV, kvalidation.DNS1123SubdomainMaxLength)
}

// DVCRCABundleConfigMapForDV returns name and namespace for ConfigMap with ca.crt.
func (g *generator) DVCRCABundleConfigMapForDV() types.NamespacedName {
	return g.generateName(tplDVCRCABundle, kvalidation.DNS1123SubdomainMaxLength)
}

// CABundleConfigMap returns name and namespace for ConfigMap which contains caBundle from dataSource.
func (g *generator) CABundleConfigMap() types.NamespacedName {
	return g.generateName(tplCABundle, kvalidation.DNS1123SubdomainMaxLength)
}

// ImagePullSecret returns name and namespace for image pull secret for the containerImage dataSource.
func (g *generator) ImagePullSecret() types.NamespacedName {
	return g.generateName(tplImagePullSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// ImporterPod generates name for importer Pod.
func (g *generator) ImporterPod() types.NamespacedName {
	return g.generateName(tplImporterPod, kvalidation.DNS1123SubdomainMaxLength)
}

// BounderPod generates name for bounder Pod.
func (g *generator) BounderPod() types.NamespacedName {
	return g.generateName(tplBounderPod, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderPod generates name for uploader Pod.
func (g *generator) UploaderPod() types.NamespacedName {
	return g.generateName(tplUploaderPod, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderService generates name for uploader Service.
func (g *generator) UploaderService() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123LabelMaxLength)
}

// UploaderIngress generates name for uploader Ingress.
func (g *generator) UploaderIngress() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// UploaderTLSSecretForIngress generates name for uploader tls secret.
func (g *generator) UploaderTLSSecretForIngress() types.NamespacedName {
	return g.generateName(tplUploaderTLSSecret, kvalidation.DNS1123SubdomainMaxLength)
}

// DataVolume generates name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *generator) DataVolume() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// NetworkPolicy generates name for NetworkPolicy.
func (g *generator) NetworkPolicy() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// CommonSupplement generates name for common supplemental resources with d8v-<prefix>-<name>-<uid> format.
// Used for snapshot-related resources (VMS Secret, VDS VolumeSnapshot).
func (g *generator) CommonSupplement() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// PersistentVolumeClaim generates name for underlying PersistentVolumeClaim.
// PVC is always one for vmd/vmi, so prefix is used.
func (g *generator) PersistentVolumeClaim() types.NamespacedName {
	return g.generateName(tplCommon, kvalidation.DNS1123SubdomainMaxLength)
}

// Legacy methods for backward compatibility

func (g *generator) shortenNamespaced(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      strings.ShortenString(name, kvalidation.DNS1123SubdomainMaxLength),
		Namespace: g.namespace,
	}
}

// LegacyDVCRAuthSecret returns old format name for auth Secret copy.
func (g *generator) LegacyDVCRAuthSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyDVCRAuthSecretForDV returns old format name for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *generator) LegacyDVCRAuthSecretForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-dv-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyDVCRCABundleConfigMapForDV returns old format name for ConfigMap with ca.crt.
func (g *generator) LegacyDVCRCABundleConfigMapForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-ca-dv-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyCABundleConfigMap returns old format name for ConfigMap which contains caBundle from dataSource.
func (g *generator) LegacyCABundleConfigMap() types.NamespacedName {
	name := fmt.Sprintf("%s-ca-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyImagePullSecret returns old format name for image pull secret for the containerImage dataSource.
func (g *generator) LegacyImagePullSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-pull-image-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyImporterPod generates old format name for importer Pod.
func (g *generator) LegacyImporterPod() types.NamespacedName {
	name := fmt.Sprintf("%s-importer-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyBounderPod generates old format name for bounder Pod.
func (g *generator) LegacyBounderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-bounder-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyUploaderPod generates old format name for uploader Pod.
func (g *generator) LegacyUploaderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyUploaderService generates old format name for uploader Service.
func (g *generator) LegacyUploaderService() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-svc-%s", g.prefix, string(g.uid))
	return g.shortenNamespaced(name)
}

// LegacyUploaderIngress generates old format name for uploader Ingress.
func (g *generator) LegacyUploaderIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-ingress-%s", g.prefix, string(g.uid))
	return g.shortenNamespaced(name)
}

// LegacyUploaderTLSSecretForIngress generates old format name for uploader tls secret.
func (g *generator) LegacyUploaderTLSSecretForIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-tls-ing-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// LegacyDataVolume generates old format name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *generator) LegacyDataVolume() types.NamespacedName {
	dvName := fmt.Sprintf("%s-%s-%s", g.prefix, g.name, string(g.uid))
	return g.shortenNamespaced(dvName)
}

// LegacyPersistentVolumeClaim generates old format name for underlying PersistentVolumeClaim.
func (g *generator) LegacyPersistentVolumeClaim() types.NamespacedName {
	return g.LegacyDataVolume()
}

// LegacySnapshotSupplement generates old format name for snapshot-related resources.
// Returns just the name without any prefix or UID (legacy naming).
func (g *generator) LegacySnapshotSupplement() types.NamespacedName {
	return types.NamespacedName{
		Name:      g.name,
		Namespace: g.namespace,
	}
}
