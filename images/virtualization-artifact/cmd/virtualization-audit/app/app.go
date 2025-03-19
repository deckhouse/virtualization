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
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	kubecache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/cache"
	"github.com/deckhouse/virtualization-controller/pkg/audit/events"
	"github.com/deckhouse/virtualization-controller/pkg/audit/informer"
	"github.com/deckhouse/virtualization-controller/pkg/audit/server"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	fs.StringVar(&o.Certfile, "tls-cert-file", "/etc/virtualization-audit/certificate/tls.crt", "Path to TLS certificate")
	fs.StringVar(&o.Keyfile, "tls-private-key-file", "/etc/virtualization-audit/certificate/tls.key", "Path to TLS key")
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

	ttlCache := cache.NewTTLCache(3 * time.Minute)

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

	vmInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines().Informer()
	vmInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			vm := obj.(*v1alpha2.VirtualMachine)
			key := fmt.Sprintf("virtualmachines/%s/%s", vm.Namespace, vm.Name)
			ttlCache.Add(key, vm)
		},
	})
	go vmInformer.Run(c.Context().Done())

	vdInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualDisks().Informer()
	go vdInformer.Run(c.Context().Done())

	vmopInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualMachineOperations().Informer()
	go vmopInformer.Run(c.Context().Done())

	podInformer := coreSharedInformerFactory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			vm := obj.(*corev1.Pod)
			key := fmt.Sprintf("pods/%s/%s", vm.Namespace, vm.Name)
			ttlCache.Add(key, vm)
		},
	})
	go podInformer.Run(c.Context().Done())

	nodeInformer := coreSharedInformerFactory.Core().V1().Nodes().Informer()
	go nodeInformer.Run(c.Context().Done())

	if !kubecache.WaitForCacheSync(
		c.Context().Done(),
		podInformer.HasSynced,
		nodeInformer.HasSynced,
		vmInformer.HasSynced,
		vdInformer.HasSynced,
		vmopInformer.HasSynced,
	) {
		return errors.New("failed to wait for caches to sync")
	}

	srv, err := server.NewServer(
		":"+opts.Port,
		events.NewVMManage(events.NewVMManageOptions{
			VMInformer:   vmInformer.GetIndexer(),
			NodeInformer: nodeInformer.GetIndexer(),
			VDInformer:   vdInformer.GetIndexer(),
			TTLCache:     ttlCache,
		}),
		events.NewVMControl(events.NewVMControlOptions{
			VMInformer:   vmInformer.GetIndexer(),
			VDInformer:   vdInformer.GetIndexer(),
			NodeInformer: nodeInformer.GetIndexer(),
			PodInformer:  podInformer.GetIndexer(),
			TTLCache:     ttlCache,
		}),
		events.NewVMOPControl(events.NewVMOPControlOptions{
			VMInformer:   vmInformer.GetIndexer(),
			VDInformer:   vdInformer.GetIndexer(),
			VMOPInformer: vmopInformer.GetIndexer(),
			NodeInformer: nodeInformer.GetIndexer(),
			TTLCache:     ttlCache,
		}),
		events.NewVMConnect(events.NewVMConnectOptions{
			VMInformer:   vmInformer.GetIndexer(),
			NodeInformer: nodeInformer.GetIndexer(),
			VDInformer:   vdInformer.GetIndexer(),
			TTLCache:     ttlCache,
		}),
		events.NewV12NControl(events.NewV12NControlOptions{
			NodeInformer: nodeInformer.GetIndexer(),
			PodInformer:  podInformer.GetIndexer(),
			TTLCache:     ttlCache,
		}),
		events.NewV12NModuleControl(events.NewV12NModuleControlOptions{
			NodeInformer: nodeInformer.GetIndexer(),
			PodInformer:  podInformer.GetIndexer(),
		}),
	)
	if err != nil {
		log.Fatal("failed to create server", log.Err(err))
	}

	// TODO: add TLS support
	return srv.Run(c.Context(), server.WithTLS(opts.Certfile, opts.Keyfile))
}
