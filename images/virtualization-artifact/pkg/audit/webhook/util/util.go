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

package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	virtv1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	admissionv1 "k8s.io/api/admission/v1"
)

func GetAdmissionReview(r *http.Request) (*admissionv1.AdmissionReview, error) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		return nil, fmt.Errorf("contentType=%s, expect application/json", contentType)
	}

	ar := &admissionv1.AdmissionReview{}
	err := json.Unmarshal(body, ar)
	return ar, err
}

func GetVMFromAdmissionReview(ar *admissionv1.AdmissionReview) (new *virtv1.VirtualMachine, old *virtv1.VirtualMachine, err error) {
	new = &virtv1.VirtualMachine{}
	err = json.Unmarshal(ar.Request.Object.Raw, new)
	if err != nil {
		return nil, nil, err
	}

	if ar.Request.Operation == admissionv1.Update {
		old = &virtv1.VirtualMachine{}
		err = json.Unmarshal(ar.Request.OldObject.Raw, old)
		if err != nil {
			return nil, nil, err
		}
	}

	return new, old, nil
}
