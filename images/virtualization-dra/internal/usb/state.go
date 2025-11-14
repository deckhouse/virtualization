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

package usb

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"k8s.io/api/resource/v1"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
)

const DefaultResyncPeriod = 30 * time.Second

func NewState(nodeName, devicesPath string, resyncPeriod time.Duration, log *slog.Logger) *State {
	if resyncPeriod == 0 {
		resyncPeriod = DefaultResyncPeriod
	}
	return &State{
		nodeName:     nodeName,
		devicesPath:  devicesPath,
		resyncPeriod: resyncPeriod,
		log:          log.With(slog.String("component", "usb-state")),
	}
}

type State struct {
	nodeName     string
	devicesPath  string
	resyncPeriod time.Duration

	log *slog.Logger

	allocatableDevices []v1.Device
	mu                 sync.RWMutex
}

func (s *State) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	{
		// TODO: remove this
		devices, err := enumerateAllPossibleDevices(10)
		if err != nil {
			return err
		}
		allocatableDevices := make([]v1.Device, 0, len(devices))
		for _, device := range devices {
			allocatableDevices = append(allocatableDevices, device)
		}
		s.allocatableDevices = allocatableDevices
	}

	return nil
}

func (s *State) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.resyncPeriod)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := s.Sync()
				if err != nil {
					s.log.Error("failed to sync usb state", slog.Any("err", err))
				}
			}
		}
	}()
	return nil
}

func (s *State) AllocatableDevices() ([]v1.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make([]v1.Device, len(s.allocatableDevices))
	for i, device := range s.allocatableDevices {
		copied[i] = *device.DeepCopy()
	}

	return copied, nil
}

func (s *State) Prepare(ctx context.Context, claim *v1.ResourceClaim) ([]*drapbv1.Device, error) {
	return nil, nil
}

func (s *State) Unprepare(ctx context.Context, claimUID string) error {
	return nil
}
