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

package util

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func WriteYamlObject(filePath string, obj client.Object) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}

	writeErr := os.WriteFile(filePath, data, 0o644)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func UnmarshalResource(filePath string, obj client.Object) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(file, obj)
	if err != nil {
		return err
	}

	return nil
}
