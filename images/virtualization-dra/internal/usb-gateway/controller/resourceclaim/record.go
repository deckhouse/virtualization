package resourceclaim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

const DefaultRecordStateDir = "/var/run/usb-gateway"

type record struct {
	Entries []Entry `json:"entries,omitempty"`
}

type Entry struct {
	Port        int    `json:"port"`
	RemotePort  int    `json:"remotePort" json:"remotePort"`
	RemoteIP    string `json:"remoteIP" json:"remoteIP"`
	RemoteBusID string `json:"remoteBusID" json:"remoteBusID"`
	BusID       string `json:"busID" json:"busID"`
}

func (e Entry) Validate() error {
	if e.Port <= 0 {
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
	return nil
}

type recordManager struct {
	recordFile string
	getter     usbip.USBInfoGetter

	mu     sync.RWMutex
	record record
}

func newRecordManager(stateDir string, getter usbip.USBInfoGetter) (*recordManager, error) {
	err := os.MkdirAll(stateDir, 0700)
	if err != nil {
		return nil, err
	}

	recordFile := filepath.Join(stateDir, "record.json")
	if _, err = os.Stat(recordFile); err != nil {
		if os.IsNotExist(err) {
			f, err := os.Create(recordFile)
			if err != nil {
				return nil, err
			}
			_ = f.Close()
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

	infos, err := r.getter.GetUsedInfo()
	if err != nil {
		return err
	}

	byBusId := make(map[string]*usbip.UsedInfo, len(infos))
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
