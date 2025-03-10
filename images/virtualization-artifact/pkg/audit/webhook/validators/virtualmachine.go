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
	"log/slog"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook/util"
)

func NewVirtualMachineWebhook(vmInformer, nodeInformer cache.Indexer) *VirtualMachineWebhook {
	return &VirtualMachineWebhook{
		vmInformer:   vmInformer,
		nodeInformer: vmInformer,
	}
}

type VirtualMachineWebhook struct {
	vmInformer   cache.Indexer
	nodeInformer cache.Indexer
}

func (m *VirtualMachineWebhook) Path() string {
	return "/virtualmachine"
}

func (m *VirtualMachineWebhook) Handler() http.Handler {
	return webhook.NewAuditWebhookHandler(m.Validate)
}

func (m *VirtualMachineWebhook) Validate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	new, _, err := util.GetVMFromAdmissionReview(ar)
	if err != nil {
		log.Error("fail to get vm from admissionReviea", log.Err(err))
	}

	nodeObj, exist, err := m.nodeInformer.Get(new.Status.Node)
	if err != nil {
		log.Error("fail to get node from informer", log.Err(err))
	}

	node, ok := nodeObj.(corev1.Node)
	if exist && ok {
		addresses := ""

		for i, r := range node.Status.Addresses {
			addresses += r.Address
			if i != len(node.Status.Addresses)-1 {
				addresses += ","
			}
		}

		log.Warn("Node", slog.String("addresses", addresses))
	}

	log.Warn(
		"VirtualMachine",
		slog.String("UID", string(new.UID)),
		slog.String("Name", new.Name),
		slog.String("Namespace", new.Namespace),
		slog.String("VirtualMachineOS", new.Status.GuestOSInfo.Name),
	)

	// 	obj, exist, err := m.vmInformer.Get(types.NamespacedName{Name: ar.Request.Name, Namespace: ar.Request.Namespace})
	// if err != nil {
	// 	log.Error("fail to get VirtualMachine from informer", log.Err(err))
	// }

	return &admissionv1.AdmissionResponse{
		AuditAnnotations: map[string]string{
			"some": "value",
		},
	}
}
