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

package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/cache"
	"github.com/deckhouse/virtualization-controller/pkg/audit/handler"
	"github.com/deckhouse/virtualization-controller/pkg/audit/informer"
	"github.com/deckhouse/virtualization-controller/pkg/audit/server"
)

const long = `
  ___  _   _______ _____ _____ ___________
 / _ \| | | |  _  \_   _|_   _|  _  | ___ \
/ /_\ \ | | | | | | | |   | | | | | | |_/ /
|  _  | | | | | | | | |   | | | | | |    /
| | | | |_| | |/ / _| |_  | | \ \_/ / |\ \
\_| |_/\___/|___/  \___/  \_/  \___/\_| \_|

Auditor is a Kubernetes auditing controller for Deckhouse Virtualization Platform
`

type Options struct {
	Port     int
	Cafile   string
	Certfile string
	Keyfile  string
	Verbose  uint8
}

func NewOptions() Options {
	return Options{}
}

func (o *Options) Flags(fs *pflag.FlagSet) {
	fs.IntVar(&o.Port, "secure-port", 8443, "The port to listen on")
	fs.StringVar(&o.Cafile, "tls-ca-file", "/etc/virtualization-audit/certificate/ca.crt", "Path to TLS CA certificate")
	fs.StringVar(&o.Certfile, "tls-cert-file", "/etc/virtualization-audit/certificate/tls.crt", "Path to TLS certificate")
	fs.StringVar(&o.Keyfile, "tls-private-key-file", "/etc/virtualization-audit/certificate/tls.key", "Path to TLS key")
	fs.Uint8VarP(&o.Verbose, "verbose", "v", 1, "verbose output")
}

func NewAuditCommand() *cobra.Command {
	opts := NewOptions()
	cmd := &cobra.Command{
		Use:   "virtualization-audit --secure-port=8443 --tls-ca-file=/ca.crt --tls-cert-file=/tls.crt --tls-private-key-file=/tls.key",
		Short: "virtualization-audit is a Kubernetes auditing controller for Deckhouse Virtualization Platform",
		Args: cobra.MatchAll(
			cobra.OnlyValidArgs,
		),
		Long:          long,
		RunE:          func(c *cobra.Command, args []string) error { return run(c, opts) },
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	opts.Flags(cmd.Flags())
	return cmd
}

func run(c *cobra.Command, opts Options) error {
	ttlCache := cache.NewTTLCache(3 * time.Minute)

	kubeCfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	client, err := kubernetes.NewForConfig(kubeCfg)
	if err != nil {
		return fmt.Errorf("unable to construct lister client: %w", err)
	}

	informerList, err := informer.NewInformerList(c.Context(), kubeCfg, ttlCache)
	if err != nil {
		return fmt.Errorf("unable to create informerList: %w", err)
	}

	err = informerList.Run(c.Context())
	if err != nil {
		return fmt.Errorf("unable to run informerList: %w", err)
	}

	eventHandler := handler.NewEventHandler(c.Context(), client, informerList, ttlCache)
	srv, err := server.NewServer(fmt.Sprintf(":%d", opts.Port), eventHandler)
	if err != nil {
		log.Fatal("failed to create server", log.Err(err))
	}

	// return srv.Run(c.Context(), server.WithTLS(opts.Cafile, opts.Certfile, opts.Keyfile))
	return srv.Run(c.Context())
}
