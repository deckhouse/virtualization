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
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type Cache interface {
	GetAddresses(k types.NamespacedName) (Addresses, bool)
	GetName(ip string) (types.NamespacedName, bool)
	Set(k types.NamespacedName, addrs Addresses)
	DeleteByKey(k types.NamespacedName)
	DeleteByIP(ip string)
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

func (c *defaultCache) GetName(ip string) (types.NamespacedName, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res, ok := c.addrVm[ip]
	return res, ok
}

func (c *defaultCache) Set(k types.NamespacedName, addrs Addresses) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vmAddr[k] = addrs
	c.addrVm[addrs.VMIP] = k
}

func (c *defaultCache) DeleteByKey(k types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	addrs, ok := c.vmAddr[k]
	if ok {
		delete(c.addrVm, addrs.VMIP)
	}
	delete(c.vmAddr, k)
}

func (c *defaultCache) DeleteByIP(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k, ok := c.addrVm[ip]
	if ok {
		delete(c.vmAddr, k)
	}
	delete(c.addrVm, ip)
}

type Addresses struct {
	NodeIP string
	VMIP   string
}
