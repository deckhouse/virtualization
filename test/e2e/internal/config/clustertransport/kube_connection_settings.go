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

package clustertransport

import (
	"fmt"
)

type ClusterTransport struct {
	KubeConfig           string `yaml:"kubeConfig"`
	Token                string `yaml:"token"`
	Endpoint             string `yaml:"endpoint"`
	CertificateAuthority string `yaml:"certificateAuthority"`
	InsecureTLS          bool   `yaml:"insecureTls"`
}

// KubeConnectionCmdSettings returns environment variables and arguments for kubectl command
// to connect to the cluster.
//
// It uses common UNIX config parameters priority:
// - First look for a command line parameter (token and endpoint in ClusterTransport).
// - environment var (CluterTransport.KubeConfig set from E2E_CLUSTERTRANSPORT_KUBECONFIG)
// - then local config file (kubeConfig from e2e config file).
// - then global config file (kubectl will use KUBECONFIG env or default config file $HOME/.kube/config).
func KubeConnectionCmdSettings(conf ClusterTransport) (env, args []string, err error) {
	// Token and endpoint have the highest priority, use them to specify connection without config file.
	if conf.Token != "" || conf.Endpoint != "" {
		if conf.Endpoint == "" {
			return nil, nil, fmt.Errorf("token is not enough, specify endpoint to connect to cluster")
		}
		if conf.Token == "" {
			return nil, nil, fmt.Errorf("endpoint is not enough, specify token to connect to cluster")
		}
		args = []string{
			fmt.Sprintf("--token=%s", conf.Token),
			fmt.Sprintf("--server=%s", conf.Endpoint),
		}
		if conf.CertificateAuthority != "" {
			args = append(args, fmt.Sprintf("--certificate-authority=%s", conf.CertificateAuthority))
		}
		if conf.InsecureTLS {
			args = append(args, "--insecure-skip-tls-verify=true")
		}

		return nil, args, nil
	}

	// Override KUBECONFIG from E2E_CLUSTERTRANSPORT_KUBECONFIG or from e2e config file.
	if conf.KubeConfig != "" {
		return []string{
			fmt.Sprintf("KUBECONFIG=%s", conf.KubeConfig),
		}, nil, nil
	}

	// No overrides from the user, let kubectl decide what to use.
	return nil, nil, nil
}
