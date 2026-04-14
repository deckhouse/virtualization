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

package progress

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

func TestStore_LoadStoreDelete(t *testing.T) {
	store := NewStore()
	uid := types.UID("vmop")
	state := State{
		Progress:       42,
		Iterative:      true,
		IterativeSince: time.Unix(100, 0),
	}

	store.Store(uid, state)

	loaded, ok := store.Load(uid)
	if !ok {
		t.Fatal("expected state to be present")
	}
	if loaded.Progress != 42 || !loaded.Iterative {
		t.Fatalf("unexpected loaded state: %+v", loaded)
	}

	store.Delete(uid)

	if _, ok := store.Load(uid); ok {
		t.Fatal("expected state to be removed")
	}
}

func TestStore_IgnoresEmptyUID(t *testing.T) {
	store := NewStore()
	store.Store("", State{Progress: 10})
	store.Delete("")

	if store.Len() != 0 {
		t.Fatalf("expected empty store, got=%d", store.Len())
	}
}
