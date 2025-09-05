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

package comp

import (
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
)

func NamespaceFlagCompletionFunc(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	directive := cobra.ShellCompDirectiveNoFileComp

	client, _, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	namespaces, err := client.CoreV1().Namespaces().List(cmd.Context(), metav1.ListOptions{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var comps []string
	for _, namespace := range namespaces.Items {
		if strings.HasPrefix(namespace.Name, toComplete) {
			comps = append(comps, namespace.Name)
		}
	}

	return comps, directive
}
