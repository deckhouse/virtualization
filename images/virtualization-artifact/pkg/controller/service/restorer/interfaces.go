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

package restorer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . ObjectHandler
type ObjectHandler interface {
	Object() client.Object
	ValidateRestore(ctx context.Context) error
	ProcessRestore(ctx context.Context) error
	ValidateClone(ctx context.Context) error
	ProcessClone(ctx context.Context) error
	Override(rules []v1alpha2.NameReplacement)
	Customize(prefix, suffix string)
}
