package uploader

import (
	"context"
	"fmt"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
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

func CleanupIngress(ctx context.Context, client client.Client, ing *netv1.Ingress) error {
	return helper.CleanupObject(ctx, client, ing)
}

func (i *Ingress) makeSpec() *netv1.Ingress {
	pathTypeExact := netv1.PathTypeExact
	path := i.generatePath()
	return &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: netv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Settings.Name,
			Namespace: i.Settings.Namespace,
			Annotations: map[string]string{
				cc.AnnUploadURL: fmt.Sprintf("https://%s%s", i.Settings.Host, path),
				"nginx.ingress.kubernetes.io/ssl-redirect":    "true",
				"nginx.ingress.kubernetes.io/proxy-body-size": "0",
				"nginx.ingress.kubernetes.io/rewrite-target":  uploadPath,
			},
			OwnerReferences: []metav1.OwnerReference{
				i.Settings.OwnerReference,
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: i.Settings.ClassName,
			Rules: []netv1.IngressRule{
				{
					Host: i.Settings.Host,
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
												Number: 443,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts:      []string{i.Settings.Host},
					SecretName: i.Settings.TLSSecretName,
				},
			},
		},
	}
}

func (i *Ingress) generatePath() string {
	return fmt.Sprintf(tmplIngressPath, util.AlphaNum(32))
}

func FindIngress(ctx context.Context, client client.Client, objName types.NamespacedName) (*netv1.Ingress, error) {
	return helper.FetchObject(ctx, objName, client, &netv1.Ingress{})
}
