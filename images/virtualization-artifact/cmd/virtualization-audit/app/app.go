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

package app

import (
	virtv1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/resource/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/server"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook/validators"
)

const long = `
  ___  _   _______ _____ _____ ___________
 / _ \| | | |  _  \_   _|_   _|  _  | ___ \
/ /_\ \ | | | | | | | |   | | | | | | |_/ /
|  _  | | | | | | | | |   | | | | | |    /
| | | | |_| | |/ / _| |_  | | \ \_/ / |\ \
\_| |_/\___/|___/  \___/  \_/  \___/\_| \_|

`

type Options struct {
	Port     string
	Certfile string
	Keyfile  string
	Verbose  uint8
}

func NewOptions() Options {
	return Options{}
}

func (o *Options) Flags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Port, "secure-port", "8443", "The port to listen on")
	fs.StringVar(&o.Certfile, "tls-cert-file", "/etc/virtulization-audit/certificate/tls.crt", "Path to TLS certificate")
	fs.StringVar(&o.Keyfile, "tls-private-key-file", "/etc/virtulization-audit/certificate/tls.key", "Path to TLS key")
	fs.Uint8VarP(&o.Verbose, "verbose", "v", 1, "verbose output")
}

func NewAuditCommand() *cobra.Command {
	opts := NewOptions()
	cmd := &cobra.Command{
		Short: "",
		Long:  long,
		RunE: func(c *cobra.Command, args []string) error {
			client, err := newClient()
			if err != nil {
				log.Fatal("failed to create k8s client", log.Err(err))
			}

			srv, err := server.NewServer(
				":"+opts.Port,
				validators.NewVirtualImageWebhook(client),
				validators.NewVirtualMachineWebhook(client),
			)
			if err != nil {
				log.Fatal("failed to create server", log.Err(err))
			}

			return srv.Run(c.Context(), server.WithTLS(opts.Certfile, opts.Keyfile))
		},
	}
	opts.Flags(cmd.Flags())
	return cmd
}

func newClient() (client.Client, error) {
	scheme := runtime.NewScheme()

	err := v1alpha2.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = virtv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	kubeCfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return client.New(
		kubeCfg,
		client.Options{Scheme: scheme},
	)
}
