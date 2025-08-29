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
	name := fmt.Sprintf("%s-dvcr-auth-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// DVCRAuthSecretForDV returns name and namespace for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *generator) DVCRAuthSecretForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-dv-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// DVCRCABundleConfigMapForDV returns name and namespace for ConfigMap with ca.crt.
func (g *generator) DVCRCABundleConfigMapForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-ca-dv-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// CABundleConfigMap returns name and namespace for ConfigMap which contains caBundle from dataSource.
func (g *generator) CABundleConfigMap() types.NamespacedName {
	name := fmt.Sprintf("%s-ca-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// ImagePullSecret returns name and namespace for image pull secret for the containerImage dataSource.
func (g *generator) ImagePullSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-pull-image-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// ImporterPod generates name for importer Pod.
func (g *generator) ImporterPod() types.NamespacedName {
	name := fmt.Sprintf("%s-importer-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// ImporterPod generates name for importer Pod.
func (g *generator) BounderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-bounder-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// UploaderPod generates name for uploader Pod.
func (g *generator) UploaderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// UploaderService generates name for uploader Service.
func (g *generator) UploaderService() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-svc-%s", g.prefix, g.uid)
	return g.shortenNamespaced(name)
}

// UploaderIngress generates name for uploader Ingress.
func (g *generator) UploaderIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-ingress-%s", g.prefix, g.uid)
	return g.shortenNamespaced(name)
}

// UploaderTLSSecretForIngress generates name for uploader tls secret.
func (g *generator) UploaderTLSSecretForIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-tls-ing-%s", g.prefix, g.name)
	return g.shortenNamespaced(name)
}

// DataVolume generates name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *generator) DataVolume() types.NamespacedName {
	dvName := fmt.Sprintf("%s-%s-%s", g.prefix, g.name, g.uid)
	return g.shortenNamespaced(dvName)
}

func (g *generator) PersistentVolumeClaim() types.NamespacedName {
	return g.DataVolume()
}

func (g *generator) shortenNamespaced(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      strings.ShortenString(name, kvalidation.DNS1123SubdomainMaxLength),
		Namespace: g.namespace,
	}
}
