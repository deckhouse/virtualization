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

package target

import (
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Kubernetes struct {
	Config       *rest.Config
	Client       *http.Client
	APIServerURL *url.URL
}

func NewKubernetesTarget() (*Kubernetes, error) {
	var err error
	k := &Kubernetes{}

	k.Config, err = config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes client config: %w", err)
	}

	// Configure HTTP client to Kubernetes API server.
	k.Client, err = rest.HTTPClientFor(k.Config)
	if err != nil {
		return nil, fmt.Errorf("setup Kubernetes API http client: %w", err)
	}

	k.APIServerURL, err = url.Parse(k.Config.Host)
	if err != nil {
		return nil, fmt.Errorf("parse API server URL: %w", err)
	}

	return k, nil
}
