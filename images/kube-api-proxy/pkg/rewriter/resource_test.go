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

package rewriter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRewriteMetadata(t *testing.T) {
	tests := []struct {
		name              string
		obj               client.Object
		newObj            client.Object
		action            Action
		expectLabels      map[string]string
		expectAnnotations map[string]string
	}{
		{
			"",
			&corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Pod",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"original.label.io":        "labelvalue",
						"original.prefix/labelkey": "labelvalue",
					},
					Annotations: map[string]string{
						"original.annotation.io": "annovalue",
					},
				},
			},
			&corev1.Pod{},
			Rename,
			map[string]string{"rewrite.label.io": "labelvalue", "rewrite.prefix/labelkey": "labelvalue"},
			map[string]string{"rewrite.annotation.io": "annovalue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.obj, "should not be nil")

			rwr := createTestRewriter()
			bytes, err := json.Marshal(tt.obj)
			require.NoError(t, err, "should marshal object %q %s/%s", tt.obj.GetObjectKind().GroupVersionKind().Kind, tt.obj.GetName(), tt.obj.GetNamespace())

			rwBytes, err := RewriteMetadata(rwr.Rules, bytes, tt.action)
			require.NoError(t, err, "should rewrite object")

			err = json.Unmarshal(rwBytes, &tt.newObj)

			require.NoError(t, err, "should unmarshal object")

			require.Equal(t, tt.newObj.GetLabels(), tt.expectLabels, "expect rewrite labels '%v' to be '%s', got '%s'", tt.obj.GetLabels(), tt.expectLabels, tt.newObj.GetLabels())
			require.Equal(t, tt.newObj.GetAnnotations(), tt.expectAnnotations, "expect rewrite annotations '%v' to be '%s', got '%s'", tt.obj.GetAnnotations(), tt.expectAnnotations, tt.newObj.GetAnnotations())
		})
	}
}
