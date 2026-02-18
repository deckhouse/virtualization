/*
Copyright 2026 Flant JSC

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

package plugin

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

// Manager is the process entry point: starts Driver (and optional HealthCheck), blocks in Run until shutdown.
type Manager struct {
	driver  *Driver
	checker *HealthCheck
}

func NewManager(driverName, nodeName string, kubeClient kubernetes.Interface, allocator Allocator, healthPort int) (*Manager, error) {
	m := &Manager{}

	driver, err := NewDriver(driverName, nodeName, kubeClient, allocator)
	if err != nil {
		return nil, err
	}
	m.driver = driver

	if healthPort > 0 {
		m.checker = NewHealthCheck(driverName, healthPort)
	}

	return m, nil
}

func (m *Manager) Run(ctx context.Context) error {
	if err := m.driver.Start(ctx); err != nil {
		return err
	}
	if m.checker != nil {
		if err := m.checker.Start(); err != nil {
			m.driver.Shutdown()
			return err
		}
	}
	m.driver.Wait()
	m.driver.Shutdown()

	if m.checker != nil {
		m.checker.Stop()
	}

	return nil
}
