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

package resourceclaim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

const DefaultRecordStateDir = "/var/run/usb-gateway"

type record struct {
	Entries []Entry `json:"entries,omitempty"`
}

type Entry struct {
	Port             int       `json:"port"`
	RemotePort       int       `json:"remotePort"`
	RemoteIP         string    `json:"remoteIP"`
	RemoteBusID      string    `json:"remoteBusID"`
	BusID            string    `json:"busID"`
	ResourceClaimUID types.UID `json:"resourceClaimUID"`
	PodUID           types.UID `json:"podUID"`
}

func (e Entry) Validate() error {
	if e.Port < 0 {
		return fmt.Errorf("port is required")
	}
	if e.RemotePort <= 0 {
		return fmt.Errorf("remotePort is required")
	}
	if e.RemoteIP == "" {
		return fmt.Errorf("remoteIP is required")
	}
	if e.RemoteBusID == "" {
		return fmt.Errorf("remoteBusID is required")
	}
	if e.BusID == "" {
		return fmt.Errorf("busID is required")
	}
	if e.ResourceClaimUID == "" {
		return fmt.Errorf("resourceClaimUID is required")
	}
	if e.PodUID == "" {
		return fmt.Errorf("podUID is required")
	}
	return nil
}

type recordManager struct {
	recordFile string
	getter     usbip.AttachInfoGetter

	mu     sync.RWMutex
	record record
}

func newRecordManager(stateDir string, getter usbip.AttachInfoGetter) (*recordManager, error) {
	err := os.MkdirAll(stateDir, 0700)
	if err != nil {
		return nil, err
	}

	recordFile := filepath.Join(stateDir, "record.json")
	if _, err = os.Stat(recordFile); err != nil {
		if os.IsNotExist(err) {
			b, err := json.Marshal(record{})
			if err != nil {
				return nil, err
			}
			err = os.WriteFile(recordFile, b, 0600)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	r := recordManager{
		recordFile: recordFile,
		getter:     getter,
	}

	if err = r.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to Refresh record: %w", err)
	}

	return &r, nil
}

func (r *recordManager) Refresh() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	infos, err := r.getter.GetAttachInfo()
	if err != nil {
		return err
	}

	byBusId := make(map[string]*usbip.AttachInfo, len(infos))
	for _, info := range infos {
		byBusId[info.LocalBusID] = &info
	}

	b, err := os.ReadFile(r.recordFile)
	if err != nil {
		return err
	}

	record := record{}
	if err = json.Unmarshal(b, &record); err != nil {
		return err
	}

	// keep only real entries
	var newEntries []Entry
	for _, e := range record.Entries {
		if _, ok := byBusId[e.RemoteBusID]; ok {
			newEntries = append(newEntries, e)
		}
	}
	record.Entries = newEntries

	r.record = record

	return nil
}

func (r *recordManager) GetEntries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.record.Entries)
}

func (r *recordManager) AddEntry(e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.record.Entries {
		if entry.RemoteBusID == e.RemoteBusID {
			return fmt.Errorf("entry with RemoteBusID %s already exists", e.RemoteBusID)
		}
		if entry.BusID == e.BusID {
			return fmt.Errorf("entry with BusID %s already exists", e.BusID)
		}
		if entry.Port == e.Port {
			return fmt.Errorf("entry with Port %d already exists", e.Port)
		}
	}

	newEntries := slices.Clone(r.record.Entries)
	newEntries = append(newEntries, e)

	record := record{Entries: newEntries}

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}

	if err = os.WriteFile(r.recordFile, b, 0600); err != nil {
		return err
	}

	r.record = record
	return nil
}
