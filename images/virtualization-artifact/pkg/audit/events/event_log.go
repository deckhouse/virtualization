package events

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apiserver/pkg/apis/audit"
)

type EventLog struct {
	Type           string `json:"type"`
	Level          string `json:"level"`
	Name           string `json:"name"`
	Datetime       string `json:"datetime"`
	Uid            string `json:"uid"`
	RequestSubject string `json:"request_subject"`

	ActionType         string `json:"action_type"`
	NodeNetworkAddress string `json:"node_network_address"`
	VirtualmachineUID  string `json:"virtualmachine_uid"`
	VirtualmachineOS   string `json:"virtualmachine_os"`
	StorageClasses     string `json:"storageclasses"`
	QemuVersion        string `json:"qemu_version"`
	LibvirtVersion     string `json:"libvirt_version"`

	OperationResult string `json:"operation_result"`
}

func NewEventLog(event *audit.Event) EventLog {
	return EventLog{
		Type:           "unknown",
		Level:          "info",
		Name:           "unknown",
		Datetime:       event.RequestReceivedTimestamp.Format(time.RFC3339),
		Uid:            string(event.AuditID),
		RequestSubject: event.User.Username,

		ActionType:         event.Verb,
		NodeNetworkAddress: "unknown",
		VirtualmachineUID:  "unknown",
		VirtualmachineOS:   "unknown",
		StorageClasses:     "unknown",
		QemuVersion:        "unknown",
		LibvirtVersion:     "unknown",

		OperationResult: event.Annotations["authorization.k8s.io/decision"],
	}
}

func (e EventLog) Log() error {
	bytes, err := json.Marshal(e)
	if err != nil {
		return err
	}

	fmt.Println(string(bytes))

	return nil
}
