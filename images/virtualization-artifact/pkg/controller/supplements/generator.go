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

// Generator calculates names for supplemental resources, e.g. ImporterPod, AuthSecret or CABundleConfigMap.
type Generator struct {
	Prefix    string
	Name      string
	Namespace string
	UID       types.UID
}

func NewGenerator(prefix, name, namespace string, uid types.UID) *Generator {
	return &Generator{
		Prefix:    prefix,
		Name:      name,
		Namespace: namespace,
		UID:       uid,
	}
}

// DVCRAuthSecret returns name and namespace for auth Secret copy.
func (g *Generator) DVCRAuthSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DVCRAuthSecretForDV returns name and namespace for auth Secret copy
// compatible with DataVolume: with accessKeyId and secretKey fields.
func (g *Generator) DVCRAuthSecretForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-auth-dv-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DVCRCABundleConfigMapForDV returns name and namespace for ConfigMap with ca.crt.
func (g *Generator) DVCRCABundleConfigMapForDV() types.NamespacedName {
	name := fmt.Sprintf("%s-dvcr-ca-dv-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// CABundleConfigMap returns name and namespace for ConfigMap which contains caBundle from dataSource.
func (g *Generator) CABundleConfigMap() types.NamespacedName {
	name := fmt.Sprintf("%s-ca-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// ImagePullSecret returns name and namespace for image pull secret for the containerImage dataSource.
func (g *Generator) ImagePullSecret() types.NamespacedName {
	name := fmt.Sprintf("%s-pull-image-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// ImporterPod generates name for importer Pod.
func (g *Generator) ImporterPod() types.NamespacedName {
	name := fmt.Sprintf("%s-importer-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// ImporterPod generates name for importer Pod.
func (g *Generator) BounderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-bounder-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// UploaderPod generates name for uploader Pod.
func (g *Generator) UploaderPod() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// UploaderService generates name for uploader Service.
func (g *Generator) UploaderService() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-svc-%s", g.Prefix, g.UID)
	return g.shortenNamespaced(name)
}

// UploaderIngress generates name for uploader Ingress.
func (g *Generator) UploaderIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-ingress-%s", g.Prefix, g.UID)
	return g.shortenNamespaced(name)
}

// UploaderTLSSecretForIngress generates name for uploader tls secret.
func (g *Generator) UploaderTLSSecretForIngress() types.NamespacedName {
	name := fmt.Sprintf("%s-uploader-tls-ing-%s", g.Prefix, g.Name)
	return g.shortenNamespaced(name)
}

// DataVolume generates name for underlying DataVolume.
// DataVolume is always one for vmd/vmi, so prefix is used.
func (g *Generator) DataVolume() types.NamespacedName {
	dvName := fmt.Sprintf("%s-%s-%s", g.Prefix, g.Name, g.UID)
	return g.shortenNamespaced(dvName)
}

func (g *Generator) PersistentVolumeClaim() types.NamespacedName {
	return g.DataVolume()
}

func (g *Generator) shortenNamespaced(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      strings.ShortenString(name, kvalidation.DNS1123SubdomainMaxLength),
		Namespace: g.Namespace,
	}
}
