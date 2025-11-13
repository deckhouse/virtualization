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

package object

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
)

func NewIsolatedProject(prefix, basePrefix string) *dv1alpha2.Project {
	return &dv1alpha2.Project{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "deckhouse.io/v1alpha2",
			Kind:       "Project",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", basePrefix, prefix),
		},
		Spec: dv1alpha2.ProjectSpec{
			ProjectTemplateName: "default",
			Parameters: map[string]interface{}{
				"administrators": []interface{}{},
				"resourceQuota": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "20",
						"memory": "20Gi",
					},
				},
				"networkPolicy": "Isolated",
			},
		},
	}
}
