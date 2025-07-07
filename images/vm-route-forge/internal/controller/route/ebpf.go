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

package route

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	vmipcache "vm-route-forge/internal/cache"
	"vm-route-forge/internal/netlinkwrap"
)

const (
	ActionAdd uint32 = iota
	ActionDelete

	KprobeFibTableInsert = "fib_table_insert"
	KprobeFibTableDelete = "fib_table_delete"
)

func NewEbpfWatcher(ctx context.Context,
	cidrs []*net.IPNet,
	routeTableID int,
	cache vmipcache.Cache,
	nlWrapper *netlinkwrap.Funcs,
	log logr.Logger,
) (*EbpfWatcher, error) {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &EbpfWatcher{
		ctx:          ctx,
		cancel:       cancel,
		result:       make(chan types.NamespacedName, defaultChanSize),
		cidrs:        cidrs,
		routeTableID: routeTableID,
		cache:        cache,
		nlWrapper:    nlWrapper,
		log:          log.WithValues("watcher", EbpfKind),
	}
	bpfMap, closeFuncs, err := w.loadEbpf()
	if err != nil {
		closeFuncs.Close()
		return nil, err
	}
	w.closeFuncs = closeFuncs
	w.bpfMap = bpfMap

	go w.watch()
	return w, nil
}

type EbpfWatcher struct {
	sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	closeFuncs   ebpfCloseFuncs
	result       chan types.NamespacedName
	cidrs        []*net.IPNet
	routeTableID int
	cache        vmipcache.Cache
	nlWrapper    *netlinkwrap.Funcs
	bpfMap       *ebpf.Map
	log          logr.Logger
}

func (w *EbpfWatcher) ResultChannel() <-chan types.NamespacedName {
	return w.result
}

func (w *EbpfWatcher) Stop() {
	w.Lock()
	defer w.Unlock()
	select {
	case <-w.ctx.Done():
	default:
		w.cancel()
		w.closeFuncs.Close()
	}
}

type ebpfCloseFuncs struct {
	items []func()
}

func (e *ebpfCloseFuncs) Add(fn func()) {
	e.items = append(e.items, fn)
}
func (e *ebpfCloseFuncs) Close() {
	for i := len(e.items) - 1; i >= 0; i-- {
		e.items[i]()
	}
}

func (w *EbpfWatcher) loadEbpf() (*ebpf.Map, ebpfCloseFuncs, error) {
	closeFuncs := ebpfCloseFuncs{}
	// Load pre-compiled programs and maps into the kernel.
	objs := ebpfObjects{}
	if err := loadEbpfObjects(&objs, nil); err != nil {
		return nil, closeFuncs, fmt.Errorf("loading objects: %w", err)
	}
	closeFuncs.Add(func() {
		if err := objs.Close(); err != nil {
			w.log.Error(err, "failed to close ebpf objects")
		}
	})

	// Open a Kprobe at the entry point of the kernel function and attach the
	// pre-compiled program. Each time the kernel function enters, the program
	// will emit an event containing pid and command of the execved task.
	kpFibTableInsert, err := link.Kprobe(KprobeFibTableInsert, objs.FibTableInsert, nil)
	if err != nil {
		return nil, closeFuncs, fmt.Errorf("opening kprobe: %w", err)
	}
	closeFuncs.Add(func() {
		if err = kpFibTableInsert.Close(); err != nil {
			w.log.Error(err, "failed to close kprobe link", "symbol", KprobeFibTableInsert)
		}
	})

	KpFibTableDelete, err := link.Kprobe(KprobeFibTableDelete, objs.FibTableDelete, nil)
	if err != nil {
		return nil, closeFuncs, fmt.Errorf("opening kprobe: %w", err)
	}
	closeFuncs.Add(func() {
		if err = KpFibTableDelete.Close(); err != nil {
			w.log.Error(err, "failed to close kprobe link", "symbol", KprobeFibTableDelete)
		}
	})

	bpfMap := objs.RouteEventsMap
	closeFuncs.Add(func() {
		if err = bpfMap.Close(); err != nil {
			w.log.Error(err, "failed to close bpf map", "type", bpfMap.Type(), "name", "RouteEventsMap")
		}
	})
	return bpfMap, closeFuncs, nil
}

func (w *EbpfWatcher) watch() {
	defer close(w.result)
	defer w.Stop()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	defer w.log.Info("Shutting down ebpf watch.")

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			for {
				var event ebpfRouteEvent
				if err := w.bpfMap.LookupAndDelete(nil, &event); err != nil {
					if !errors.Is(err, ebpf.ErrKeyNotExist) {
						w.log.Error(err, "Failed to lookup and delete key.", "event", event)
					}
					break
				}
				w.log.V(7).Info("Received a new ebpf event", "event", event)
				if err := w.sync(event); err != nil {
					w.log.Error(err, "Failed to sync ebpf event.", "event", event)
				}
			}
		}
	}
}

func (w *EbpfWatcher) sync(event ebpfRouteEvent) error {
	if event.Table != uint32(w.routeTableID) || event.Dst == 0 {
		return nil
	}

	vmIP := ipUint32ToNetIP(event.Dst)
	isManaged, err := isManagedIP(vmIP, w.cidrs)
	if err != nil {
		return err
	}
	if !isManaged {
		return nil
	}

	key, found := w.cache.GetName(vmIP)
	// if the route was added but not added to cache, then do nothing, because we can't get name of vm.
	if !found {
		return nil
	}

	log := w.log.WithValues(
		"inHostVMIP", vmIP.String(),
		"virtualMachine", key)
	log.Info("Started processing route")

	switch event.Action {
	case ActionAdd:
		addrs, found := w.cache.GetAddresses(key)
		if !found {
			log.Info("The route was added, but there are no addresses in the cache. Add the VM to the queue.")
			w.enqueueKey(key)
			break
		}
		if event.Src == 0 {
			return fmt.Errorf("wrong src in ebpf event")
		}
		ciliumInternalIP := ipUint32ToNetIP(event.Src)

		routes, err := w.nlWrapper.RouteGet(addrs.NodeIP.NetIP())
		if err != nil || len(routes) == 0 {
			return fmt.Errorf("failed to get routes: %w", err)
		}
		ciliumInternalIPByNodeIP := routes[0].Src

		if !ciliumInternalIP.Equal(ciliumInternalIPByNodeIP) || !addrs.VMIP.NetIP().Equal(vmIP) {
			log.Info("The route was added, but the addresses from the cache and from the route do not match. Add the VM to the queue.",
				"inCacheNodeIP", addrs.NodeIP.String(),
				"inCacheVMIP", addrs.VMIP.String(),
				"ciliumInternalIP", ciliumInternalIP.String(),
				"ciliumInternalIPByNodeIP", ciliumInternalIPByNodeIP.String(),
			)
			w.enqueueKey(key)
		}
	case ActionDelete:
		log.Info("The route was deleted but not deleted from the cache. Add the VM to the queue.")
		w.enqueueKey(key)
	default:
		return fmt.Errorf("invalid action: %v", event.Action)
	}
	return nil
}

func (w *EbpfWatcher) enqueueKey(key types.NamespacedName) {
	w.result <- key
}

// Converts uint32 IP address in little-endian format to a human-readable string format
func ipUint32ToNetIP(ip uint32) net.IP {
	ipBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(ipBytes, ip)
	return ipBytes
}
