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

package validators

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVirtualImageWebhook(client client.Client) *VirtualImageWebhook {
	return &VirtualImageWebhook{client}
}

type VirtualImageWebhook struct {
	client client.Client
}

func (m *VirtualImageWebhook) Path() string {
	return "/virtualimage"
}

func (m *VirtualImageWebhook) Handler() http.Handler {
	return webhook.NewAuditWebhookHandler(m.Validate)
}

func (m *VirtualImageWebhook) Validate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	response := &admissionv1.AdmissionResponse{
		AuditAnnotations: map[string]string{},
	}

	obj, err := object.FetchObject(
		context.Background(),
		types.NamespacedName{Name: ar.Request.Name, Namespace: ar.Request.Namespace},
		m.client,
		&virtv1.VirtualImage{},
	)
	if err != nil {
		log.Error("fail to fetch object", log.Err(err))
	}

	fmt.Printf("%#v\n\n", obj)

	log.Warn(
		"virtualimage",
		slog.String("name", obj.Name),
		slog.String("namespace", obj.Namespace),
		slog.String("storage", string(obj.Spec.Storage)),
		slog.String("dataSourceType", string(obj.Spec.DataSource.Type)),
		slog.String("storageClass", *obj.Spec.PersistentVolumeClaim.StorageClass),
	)

	response.AuditAnnotations["some"] = "value"

	return response
}
