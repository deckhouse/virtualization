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

package restorer

import (
	"errors"
	"sync"

	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/content"
)

type (
	ContentClient = content.Client
	RawManifest   = content.RawManifest
)

func NewContentClient(cfg *rest.Config) (*ContentClient, error) {
	return content.NewClient(cfg)
}

var (
	contentClientMu sync.RWMutex
	contentClient   *ContentClient
)

func InitContentClient(cfg *rest.Config) error {
	cc, err := NewContentClient(cfg)
	if err != nil {
		return err
	}

	contentClientMu.Lock()
	defer contentClientMu.Unlock()
	contentClient = cc
	return nil
}

// sharedContentClient returns the client InitContentClient installed.
func sharedContentClient() (*ContentClient, error) {
	contentClientMu.RLock()
	defer contentClientMu.RUnlock()

	if contentClient == nil {
		return nil, errors.New("state-snapshotter content client is not initialized: call restorer.InitContentClient at startup")
	}
	return contentClient, nil
}
