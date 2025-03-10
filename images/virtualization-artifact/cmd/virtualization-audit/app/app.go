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
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/informer"
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
		RunE:  func(c *cobra.Command, args []string) error { return run(c, opts) },
	}
	opts.Flags(cmd.Flags())
	return cmd
}

func run(c *cobra.Command, opts Options) error {
	kubeCfg, err := config.GetConfig()
	if err != nil {
		return err
	}

	virtSharedInformerFactory, err := informer.VirtualizationInformerFactory(kubeCfg)
	if err != nil {
		log.Error("failed to create virtualization shared factory", log.Err(err))
		return err
	}

	coreSharedInformerFactory, err := informer.CoreInformerFactory(kubeCfg)
	if err != nil {
		log.Error("failed to create core shared factory", log.Err(err))
		return err
	}

	if virtSharedInformerFactory == nil {
		return errors.New("virt factory nil")
	}

	if coreSharedInformerFactory == nil {
		return errors.New("core factory nil")
	}

	vmInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines().Informer()
	go vmInformer.Run(c.Context().Done())

	nodeInformer := coreSharedInformerFactory.Core().V1().Nodes().Informer()
	go nodeInformer.Run(c.Context().Done())

	// Ensure cache is up-to-date
	ok := cache.WaitForCacheSync(c.Context().Done(), nodeInformer.HasSynced, vmInformer.HasSynced)
	if !ok {
		return nil
	}

	srv, err := server.NewServer(
		":"+opts.Port,
		validators.NewVirtualMachineWebhook(vmInformer.GetIndexer(), nodeInformer.GetIndexer()),
	)
	if err != nil {
		log.Fatal("failed to create server", log.Err(err))
	}

	return srv.Run(c.Context(), server.WithTLS(opts.Certfile, opts.Keyfile))
}
