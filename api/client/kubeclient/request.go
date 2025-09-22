/*
Copyright 2018 The KubeVirt Authors
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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/staging/src/kubevirt.io/client-go/kubecli/vmi.go
*/

package kubeclient

import (
	"fmt"
	"net/http"
	"net/url"
	"path"

	"k8s.io/client-go/rest"
)

const subresourceURLTpl = "/apis/subresources.virtualization.deckhouse.io/v1alpha3/namespaces/%s/%s/%s/%s"

func RequestFromConfig(config *rest.Config, resource, name, namespace, subresource string,
	queryParams url.Values) (*http.Request, error) {
	u, err := url.Parse(config.Host)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("unsupported Protocol %s", u.Scheme)
	}

	u.Path = path.Join(
		u.Path,
		fmt.Sprintf(subresourceURLTpl, namespace, resource, name, subresource),
	)
	if len(queryParams) > 0 {
		u.RawQuery = queryParams.Encode()
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: map[string][]string{},
	}

	return req, nil
}
