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

package uploader

import (
	"context"
	"fmt"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
)

type IngressSettings struct {
	Name           string
	Namespace      string
	Host           string
	TLSSecretName  string
	ServiceName    string
	ClassName      *string
	OwnerReference metav1.OwnerReference
}

const (
	tmplIngressPath = "/upload/%s"
	uploadPath      = "/upload"
)

type Ingress struct {
	Settings *IngressSettings
}

func NewIngress(settings *IngressSettings) *Ingress {
	return &Ingress{settings}
}

func (i *Ingress) Create(ctx context.Context, client client.Client) (*netv1.Ingress, error) {
	ing := i.makeSpec()

	if err := client.Create(ctx, ing); err != nil {
		return nil, err
	}

	return ing, nil
}

// makeSpec fills Ingress structure with uploader settings.
//
// Notes:
//   - AnnUploadURL annotation is used by VI/CVI handlers to show URL for external upload.
//   - AnnUploadPath annotation is a workaround to support clusters without publicDomainTemplate.
func (i *Ingress) makeSpec() *netv1.Ingress {
	pathTypeExact := netv1.PathTypeExact
	path := i.generatePath()
	tlsEnabled := i.Settings.TLSSecretName != ""
	uploadHost := "dvcr-upload"
	uploadURL := ""
	if i.Settings.Host != "" {
		uploadHost = i.Settings.Host
		if tlsEnabled {
			uploadURL = fmt.Sprintf("https://%s%s", i.Settings.Host, path)
		} else {
			uploadURL = fmt.Sprintf("http://%s%s", i.Settings.Host, path)
		}
	}
	ingress := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: netv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Settings.Name,
			Namespace: i.Settings.Namespace,
			Annotations: map[string]string{
				annotations.AnnUploadURL:                              uploadURL,
				annotations.AnnUploadPath:                             path,
				"nginx.ingress.kubernetes.io/proxy-body-size":         "0",
				"nginx.ingress.kubernetes.io/proxy-request-buffering": "off",
				"nginx.ingress.kubernetes.io/proxy-buffering":         "off",
				"nginx.ingress.kubernetes.io/rewrite-target":          uploadPath,
			},
			OwnerReferences: []metav1.OwnerReference{
				i.Settings.OwnerReference,
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: i.Settings.ClassName,
			Rules: []netv1.IngressRule{
				{
					Host: uploadHost,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathTypeExact,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: i.Settings.ServiceName,
											Port: netv1.ServiceBackendPort{
												Number: common.UploaderPort,
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

	if tlsEnabled {
		ingress.Spec.TLS = []netv1.IngressTLS{
			{
				Hosts:      []string{uploadHost},
				SecretName: i.Settings.TLSSecretName,
			},
		}
		ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	}

	return ingress
}

func (i *Ingress) generatePath() string {
	return fmt.Sprintf(tmplIngressPath, pwgen.AlphaNum(32))
}
