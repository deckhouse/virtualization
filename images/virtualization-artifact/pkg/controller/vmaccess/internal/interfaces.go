/*
Copyright 2026 Flant JSC

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

package internal

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

//go:generate go tool moq -rm -out mock.go . Freezer

// Freezer freezes and unfreezes the guest filesystem on behalf of a claim.
type Freezer interface {
	Freeze(ctx context.Context, namespace, name string, claimUID types.UID, unfreezeTimeout time.Duration) error
	Unfreeze(ctx context.Context, namespace, name string, claimUID types.UID) error
}
