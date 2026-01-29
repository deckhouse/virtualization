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

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const USBGatewayStatusKind = "USBGatewayStatus"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type USBGatewayStatus struct {
	metav1.TypeMeta `json:",inline"`

	BusID string `json:"busID"`

	RemoteIP   string `json:"remoteIP"`
	RemotePort int    `json:"remotePort"`

	Bound    bool `json:"bound"`
	Attached bool `json:"attached"`
}

func FromData(data *runtime.RawExtension) (*USBGatewayStatus, error) {
	if data == nil {
		return nil, nil
	}

	obj, err := runtime.Decode(Codecs.UniversalDecoder(SchemeGroupVersion), data.Raw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode USBGatewayStatus: %w", err)
	}
	status, ok := obj.(*USBGatewayStatus)
	if !ok {
		return nil, fmt.Errorf("failed to decode USBGatewayStatus: unexpected object type: %T", obj)
	}

	return status, nil
}

func ToData(status *USBGatewayStatus) (*runtime.RawExtension, error) {
	if status == nil {
		return nil, nil
	}

	raw, err := runtime.Encode(Codecs.LegacyCodec(SchemeGroupVersion), status)
	if err != nil {
		return nil, fmt.Errorf("failed to encode USBGatewayStatus: %w", err)
	}

	return &runtime.RawExtension{
		Raw:    raw,
		Object: status,
	}, nil
}
