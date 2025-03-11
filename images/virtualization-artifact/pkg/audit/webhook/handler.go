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

package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook/util"
)

type validator func(ar *admissionv1.AdmissionReview) (*admissionv1.AdmissionResponse, error)

func NewAuditWebhookHandler(validate validator) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		review, err := util.GetAdmissionReview(req)
		if err != nil {
			resp.WriteHeader(http.StatusBadRequest)
			return
		}

		response := admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				APIVersion: admissionv1.SchemeGroupVersion.String(),
				Kind:       "AdmissionReview",
			},
		}

		reviewResponse, err := validate(review)
		if err != nil {
			log.Error(
				"fail to validate",
				log.Err(err),
				slog.String("UID", string(review.Request.UID)),
				slog.String("name", review.Request.Name),
				slog.String("namespace", review.Request.Namespace),
				slog.String("kind", review.Request.Kind.Kind),
				slog.String("resource", review.Request.Resource.Resource),
				slog.String("operation", string(review.Request.Operation)),
			)
		}

		if reviewResponse != nil {
			response.Response = reviewResponse
			response.Response.Allowed = true
			response.Response.UID = review.Request.UID
		}
		// reset the Object and OldObject, they are not needed in a response.
		review.Request.Object = runtime.RawExtension{}
		review.Request.OldObject = runtime.RawExtension{}

		responseBytes, err := json.Marshal(response)
		if err != nil {
			slog.Default().Error("Failed to marshal webhook response")
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := resp.Write(responseBytes); err != nil {
			slog.Default().Error("Failed to write webhook response")
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
	})
}
