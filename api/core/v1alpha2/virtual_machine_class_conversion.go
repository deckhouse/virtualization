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

package v1alpha2

import (
	"fmt"
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

var _ conversion.Hub = &VirtualMachineClass{}

func (*VirtualMachineClass) Hub() {}

// UnmarshalJSON accepts the v1alpha3 wire form ("5%") next to the native number.
// A v1alpha3 client can write an object while the CRD has no conversion webhook yet
// (the module is being installed), and the apiserver then stores that body as v1alpha2
// verbatim. One such object must not break every v1alpha2 reader of the collection.
func (v *CoreFractionValue) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if raw == "null" {
		return nil
	}

	value, err := strconv.Atoi(strings.TrimSuffix(strings.Trim(raw, `"`), "%"))
	if err != nil {
		return fmt.Errorf("coreFraction must be a number or a percentage string, got %s", raw)
	}

	*v = CoreFractionValue(value)
	return nil
}
