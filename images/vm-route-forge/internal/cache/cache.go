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

package cache

import (
	"net"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type Cache interface {
	GetAddresses(k types.NamespacedName) (Addresses, bool)
	GetName(ip net.IP) (types.NamespacedName, bool)
	Set(k types.NamespacedName, addrs Addresses)
	DeleteByKey(k types.NamespacedName)
	DeleteByIP(ip net.IP)
	Iterate(fn func(key types.NamespacedName, v Addresses) (next bool))
}

func NewCache() Cache {
	return &defaultCache{
		vmAddr: make(map[types.NamespacedName]Addresses),
		addrVm: make(map[string]types.NamespacedName),
	}
}

type defaultCache struct {
	mu     sync.RWMutex
	vmAddr map[types.NamespacedName]Addresses
	addrVm map[string]types.NamespacedName
}

func (c *defaultCache) GetAddresses(k types.NamespacedName) (Addresses, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res, ok := c.vmAddr[k]
	return res, ok
}

func (c *defaultCache) GetName(ip net.IP) (types.NamespacedName, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res, ok := c.addrVm[ip.String()]
	return res, ok
}

func (c *defaultCache) Set(k types.NamespacedName, addrs Addresses) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vmAddr[k] = addrs
	c.addrVm[addrs.VMIP.String()] = k
}

func (c *defaultCache) DeleteByKey(k types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	addrs, ok := c.vmAddr[k]
	if ok {
		delete(c.addrVm, addrs.VMIP.String())
	}
	delete(c.vmAddr, k)
}

func (c *defaultCache) DeleteByIP(ip net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k, ok := c.addrVm[ip.String()]
	if ok {
		delete(c.vmAddr, k)
	}
	delete(c.addrVm, ip.String())
}

func (c *defaultCache) Iterate(fn func(k types.NamespacedName, v Addresses) (next bool)) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k, v := range c.vmAddr {
		if next := fn(k, v); !next {
			break
		}
	}
}

type Addresses struct {
	NodeIP IP
	VMIP   IP
}

type IP string

func (ip IP) String() string {
	return string(ip)
}

func (ip IP) NetIP() net.IP {
	return net.ParseIP(string(ip))
}
