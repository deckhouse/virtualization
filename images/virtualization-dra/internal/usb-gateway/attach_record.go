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

package usbgateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

const DefaultRecordStateDir = "/var/run/virtualization-dra/usb"

type attachRecord struct {
	Entries []AttachEntry `json:"entries,omitempty"`
}

type AttachEntry struct {
	Rhport     int    `json:"rhport"`
	BusID      string `json:"busID"`
	DeviceName string `json:"deviceName"`
}

func (e AttachEntry) Validate() error {
	if e.Rhport < 0 {
		return fmt.Errorf("rhport is required")
	}
	if e.BusID == "" {
		return fmt.Errorf("busID is required")
	}
	if e.DeviceName == "" {
		return fmt.Errorf("deviceName is required")
	}
	return nil
}

type attachRecordManager struct {
	recordFile string
	getter     usbip.AttachInfoGetter

	mu     sync.RWMutex
	record attachRecord
}

func newAttachRecordManager(stateDir string, getter usbip.AttachInfoGetter) (*attachRecordManager, error) {
	err := os.MkdirAll(stateDir, 0o700)
	if err != nil {
		return nil, err
	}

	recordFile := filepath.Join(stateDir, "attach-record.json")
	if _, err = os.Stat(recordFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		b, err := json.Marshal(attachRecord{})
		if err != nil {
			return nil, err
		}
		err = os.WriteFile(recordFile, b, 0o600)
		if err != nil {
			return nil, err
		}
	}

	r := attachRecordManager{
		recordFile: recordFile,
		getter:     getter,
	}

	if err = r.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to Refresh record: %w", err)
	}

	return &r, nil
}

func (r *attachRecordManager) Refresh() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := r.getter.GetAttachInfo()
	if err != nil {
		return err
	}

	ports := make(map[int]struct{}, len(info.Items))
	for _, item := range info.Items {
		ports[item.Port] = struct{}{}
	}

	b, err := os.ReadFile(r.recordFile)
	if err != nil {
		return err
	}

	record := attachRecord{}
	if err = json.Unmarshal(b, &record); err != nil {
		return err
	}

	// keep only real entries
	var newEntries []AttachEntry
	for _, e := range record.Entries {
		if _, ok := ports[e.Rhport]; ok {
			newEntries = append(newEntries, e)
		}
	}

	record.Entries = newEntries

	r.record = record

	return nil
}

func (r *attachRecordManager) GetEntries() []AttachEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.record.Entries)
}

func (r *attachRecordManager) AddEntry(e AttachEntry) error {
	if err := e.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.record.Entries {
		if entry.Rhport == e.Rhport {
			return fmt.Errorf("entry with Rhport %d already exists", e.Rhport)
		}
		if entry.BusID == e.BusID {
			return fmt.Errorf("entry with BusID %s already exists", e.BusID)
		}
		if entry.DeviceName == e.DeviceName {
			return fmt.Errorf("entry with DeviceName %s already exists", e.DeviceName)
		}
	}

	newEntries := slices.Clone(r.record.Entries)
	newEntries = append(newEntries, e)

	record := attachRecord{Entries: newEntries}

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}

	if err = os.WriteFile(r.recordFile, b, 0o600); err != nil {
		return err
	}

	r.record = record
	return nil
}
