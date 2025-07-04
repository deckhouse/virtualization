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

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
)

const (
	tmplIngressPath = "/download/%s"
	downloadPath    = "/download"
)

func (d defaultFactory) Ingress() *netv1.Ingress {
	pathTypeExact := netv1.PathTypeExact
	path := d.generatePath()

	ing := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: netv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.sup.ExporterIngress().Name,
			Namespace: d.sup.ExporterIngress().Namespace,
			Annotations: map[string]string{
				annotations.AnnExportURL:                              fmt.Sprintf("https://%s%s", d.host, path),
				"nginx.ingress.kubernetes.io/ssl-redirect":            "true",
				"nginx.ingress.kubernetes.io/proxy-body-size":         "0",
				"nginx.ingress.kubernetes.io/proxy-request-buffering": "off",
				"nginx.ingress.kubernetes.io/proxy-buffering":         "off",
				"nginx.ingress.kubernetes.io/rewrite-target":          downloadPath,
			},
			OwnerReferences: []metav1.OwnerReference{
				d.makeOwnerReference(),
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: d.className,
			Rules: []netv1.IngressRule{
				{
					Host: d.host,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathTypeExact,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: d.sup.ExporterService().Name,
											Port: netv1.ServiceBackendPort{
												Name: exporterPortName,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if d.tlsSecretName != nil {
		ing.Spec.TLS = []netv1.IngressTLS{
			{
				Hosts:      []string{d.host},
				SecretName: *d.tlsSecretName,
			},
		}
	}

	return ing
}

func (d defaultFactory) generatePath() string {
	return fmt.Sprintf(tmplIngressPath, pwgen.AlphaNum(32))
}
