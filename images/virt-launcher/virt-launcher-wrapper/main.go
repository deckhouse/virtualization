package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

type LogMessage struct {
	Msg       string `json:"msg"`
	Level     string `json:"level"`
	Component string `json:"component"`
}

// logMessage formats and prints structured logs
func logMessage(msg, level, component string) {
	logEntry := LogMessage{
		Msg:       msg,
		Level:     level,
		Component: component,
	}
	jsonLog, _ := json.Marshal(logEntry)
	fmt.Println(string(jsonLog))
}

// getRunningVM fetches the name of the running VM
func getRunningVM() string {
	for {
		out, err := exec.Command("virsh", "list", "--name").Output()
		if err == nil {
			vmName := string(out)
			if len(vmName) > 0 {
				return vmName
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// setDomainRebootAction sets the reboot action for the VM
func setDomainRebootAction(vmName string) {
	logMessage(fmt.Sprintf("Set reboot action to shutdown for domain %s", vmName), "info", "domain-monitor")
	cmd := exec.Command("virsh", "qemu-monitor-command", vmName,
		`{"execute": "set-action", "arguments":{"reboot":"shutdown"}}`)
	if err := cmd.Run(); err != nil {
		log.Printf("Failed to set reboot action: %v\n", err)
	}
}

// monitorDomainEvents listens for domain shutdown events
func monitorDomainEvents(vmName string) {
	logMessage(fmt.Sprintf("Monitor domain %s events", vmName), "info", "domain-monitor")
	cmd := exec.Command("virsh", "qemu-monitor-event", "--domain", vmName, "--loop", "--event", "SHUTDOWN")
	file, err := os.Create("/dev/termination-log")
	if err != nil {
		log.Fatalf("Failed to open termination log: %v\n", err)
	}
	defer file.Close()

	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("Failed to monitor domain events: %v\n", err)
	}
}

func main() {
	logMessage("Start domain monitor daemon", "info", "virt-launcher-monitor-wrapper")

	// Start domain monitor in a separate goroutine
	go func() {
		vmName := getRunningVM()
		setDomainRebootAction(vmName)
		monitorDomainEvents(vmName)
	}()

	// Check and execute the original `virt-launcher-monitor-orig` if present
	origMonitorPath := "/usr/bin/virt-launcher-monitor-orig"
	if _, err := os.Stat(origMonitorPath); os.IsNotExist(err) {
		logMessage("Target /usr/bin/virt-launcher-monitor-orig is absent", "error", "virt-launcher-monitor-wrapper")
		os.Exit(1)
	}

	logMessage("Exec original virt-launcher-monitor", "info", "virt-launcher-monitor-wrapper")

	// Execute the original virt-launcher-monitor with passed arguments
	cmd := exec.Command(origMonitorPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to execute %s: %v\n", origMonitorPath, err)
	}
}
