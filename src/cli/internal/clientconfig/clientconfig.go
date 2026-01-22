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

package clientconfig

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

type key struct{}

var clientConfigKey key

// NewContext returns a new Context that stores a clientConfig as value.
func NewContext(ctx context.Context, clientConfig clientcmd.ClientConfig) context.Context {
	return context.WithValue(ctx, clientConfigKey, clientConfig)
}

func ClientAndNamespaceFromContext(ctx context.Context) (client kubeclient.Client, namespace string, overridden bool, err error) {
	clientConfig, ok := ctx.Value(clientConfigKey).(clientcmd.ClientConfig)
	if !ok {
		return nil, "", false, fmt.Errorf("unable to get client config from context")
	}
	client, err = kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		return nil, "", false, err
	}
	namespace, overridden, err = clientConfig.Namespace()
	if err != nil {
		return nil, "", false, err
	}
	return client, namespace, overridden, nil
}

func GetRESTConfig(ctx context.Context) (*rest.Config, error) {
	clientConfig, ok := ctx.Value(clientConfigKey).(clientcmd.ClientConfig)
	if !ok {
		return nil, fmt.Errorf("unable to get client config from context")
	}
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return config, nil
}
