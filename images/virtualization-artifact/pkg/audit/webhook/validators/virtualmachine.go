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
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook"
)

func NewVirtualMachineWebhook(client client.Client) *VirtualMachineWebhook {
	return &VirtualMachineWebhook{client}
}

type VirtualMachineWebhook struct {
	client client.Client
}

func (m *VirtualMachineWebhook) Path() string {
	return "/virtualmachine"
}

func (m *VirtualMachineWebhook) Handler() http.Handler {
	return webhook.NewAuditWebhookHandler(m.Validate)
}

func (m *VirtualMachineWebhook) Validate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	response := &admissionv1.AdmissionResponse{
		AuditAnnotations: map[string]string{},
	}

	response.AuditAnnotations["some"] = "value"

	return response
}
