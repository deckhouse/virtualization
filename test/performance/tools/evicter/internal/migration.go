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

package internal

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationState tracks the state of a VM migration
type MigrationState struct {
	VMName     string
	StartTime  time.Time
	VMOPName   string
	IsComplete bool
}

// ContinuousMigrator manages continuous VM migrations
type ContinuousMigrator struct {
	client           kubeclient.Client
	namespace        string
	targetPercentage int
	runDuration      time.Duration
	migratingVMs     map[string]*MigrationState
	mutex            sync.RWMutex
	stopChan         chan struct{}
	doneChan         chan struct{}
}

// NewContinuousMigrator creates a new continuous migrator
func NewContinuousMigrator(client kubeclient.Client, namespace string, targetPercentage int, runDuration time.Duration) *ContinuousMigrator {
	return &ContinuousMigrator{
		client:           client,
		namespace:        namespace,
		targetPercentage: targetPercentage,
		runDuration:      runDuration,
		migratingVMs:     make(map[string]*MigrationState),
		stopChan:         make(chan struct{}),
		doneChan:         make(chan struct{}),
	}
}

// StartContinuousMigrator starts the continuous migration process
func StartContinuousMigrator(client kubeclient.Client, namespace string, targetPercentage int, runDuration time.Duration) {
	migrator := NewContinuousMigrator(client, namespace, targetPercentage, runDuration)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the migrator in a goroutine
	go migrator.run()

	// Setup timeout if specified
	if runDuration > 0 {
		go func() {
			time.Sleep(runDuration)
			slog.Info("Migration timeout reached, stopping migrator")
			close(migrator.stopChan)
		}()
	}

	// Wait for signal or completion
	select {
	case <-sigChan:
		slog.Info("Received interrupt signal, stopping migrator gracefully...")
		close(migrator.stopChan)
	case <-migrator.doneChan:
		slog.Info("Migration process completed")
	}

	// Wait for graceful shutdown
	<-migrator.doneChan
	slog.Info("Migrator stopped")
}

// run is the main migration loop
func (m *ContinuousMigrator) run() {
	defer close(m.doneChan)

	slog.Info("Starting continuous migrator",
		"namespace", m.namespace,
		"targetPercentage", m.targetPercentage,
		"duration", m.runDuration)

	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			slog.Info("Stop signal received, shutting down...")
			return
		case <-ticker.C:
			m.checkAndStartMigrations()
			m.monitorMigrations()
		}
	}
}

// checkAndStartMigrations checks if we need to start new migrations
func (m *ContinuousMigrator) checkAndStartMigrations() {
	// Get all VMs in the namespace
	vmList, err := m.client.VirtualMachines(m.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to list VMs", "error", err)
		return
	}

	// Filter VMs that are running and not currently migrating
	var availableVMs []v1alpha2.VirtualMachine
	for _, vm := range vmList.Items {
		// Only consider VMs that are in Running state and not already migrating
		if vm.Status.Phase == v1alpha2.MachineRunning {
			m.mutex.RLock()
			_, isMigrating := m.migratingVMs[vm.Name]
			m.mutex.RUnlock()

			if !isMigrating {
				availableVMs = append(availableVMs, vm)
			}
		}
	}

	// Calculate how many VMs should be migrating
	targetCount := (m.targetPercentage * len(vmList.Items)) / 100
	currentMigrating := len(m.migratingVMs)

	// Start new migrations if needed
	if currentMigrating < targetCount && len(availableVMs) > 0 {
		needed := targetCount - currentMigrating
		if needed > len(availableVMs) {
			needed = len(availableVMs)
		}

		// Shuffle and select VMs to migrate
		rand.Shuffle(len(availableVMs), func(i, j int) {
			availableVMs[i], availableVMs[j] = availableVMs[j], availableVMs[i]
		})

		for i := 0; i < needed; i++ {
			m.startMigration(availableVMs[i])
		}
	}

	slog.Info("Migration status",
		"totalVMs", len(vmList.Items),
		"targetCount", targetCount,
		"currentMigrating", currentMigrating,
		"availableVMs", len(availableVMs))
}

// startMigration starts a migration for a VM
func (m *ContinuousMigrator) startMigration(vm v1alpha2.VirtualMachine) {
	// Double-check that VM is still available for migration
	ctx := context.TODO()
	currentVM, err := m.client.VirtualMachines(m.namespace).Get(ctx, vm.Name, metav1.GetOptions{})
	if err != nil {
		slog.Error("Failed to get current VM status", "vm", vm.Name, "error", err)
		return
	}

	// Check if VM is still in Running state and not migrating
	if currentVM.Status.Phase != v1alpha2.MachineRunning {
		slog.Info("VM is no longer in Running state, skipping migration",
			"vm", vm.Name,
			"currentPhase", currentVM.Status.Phase)
		return
	}

	// Check if VM is already being tracked as migrating
	m.mutex.RLock()
	_, isMigrating := m.migratingVMs[vm.Name]
	m.mutex.RUnlock()

	if isMigrating {
		slog.Info("VM is already being migrated, skipping", "vm", vm.Name)
		return
	}

	// Check if VM already has an active VMOP in the cluster
	vmopList, err := m.client.VirtualMachineOperations(m.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to check existing VMOPs", "vm", vm.Name, "error", err)
		return
	}

	// Check if there are any active VMOPs for this VM
	for _, vmop := range vmopList.Items {
		if vmop.Spec.VirtualMachine == vm.Name {
			if vmop.Status.Phase == v1alpha2.VMOPPhaseInProgress ||
				vmop.Status.Phase == v1alpha2.VMOPPhasePending {
				slog.Info("VM already has active VMOP, skipping migration",
					"vm", vm.Name,
					"vmop", vmop.Name,
					"phase", vmop.Status.Phase)
				return
			}
		}
	}

	vmop := &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vm.Name + "-migrate-",
			Namespace:    m.namespace,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           v1alpha2.VMOPTypeMigrate,
			VirtualMachine: vm.Name,
		},
	}

	createdVMOP, err := m.client.VirtualMachineOperations(m.namespace).Create(ctx, vmop, metav1.CreateOptions{})
	if err != nil {
		slog.Error("Failed to create VMOP", "vm", vm.Name, "error", err)
		return
	}

	// Track the migration
	m.mutex.Lock()
	m.migratingVMs[vm.Name] = &MigrationState{
		VMName:    vm.Name,
		StartTime: time.Now(),
		VMOPName:  createdVMOP.Name,
	}
	m.mutex.Unlock()

	slog.Info("Started migration", "vm", vm.Name, "vmop", createdVMOP.Name)
}

// monitorMigrations monitors ongoing migrations
func (m *ContinuousMigrator) monitorMigrations() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.TODO()

	for vmName, state := range m.migratingVMs {
		if state.IsComplete {
			continue
		}

		// Get current VM status
		vm, err := m.client.VirtualMachines(m.namespace).Get(ctx, vmName, metav1.GetOptions{})
		if err != nil {
			slog.Error("Failed to get VM status", "vm", vmName, "error", err)
			continue
		}

		// Check if VM is in error state
		if vm.Status.Phase == v1alpha2.MachineDegraded {
			slog.Warn("VM is in degraded state, removing from migration tracking",
				"vm", vmName,
				"vmop", state.VMOPName)
			delete(m.migratingVMs, vmName)
			continue
		}

		// Check for migration timeout (5 minutes)
		if time.Since(state.StartTime) > 5*time.Minute {
			slog.Warn("Migration timeout reached, removing from tracking",
				"vm", vmName,
				"duration", time.Since(state.StartTime),
				"vmop", state.VMOPName)
			delete(m.migratingVMs, vmName)
			continue
		}

		// Check if migration is complete
		if m.isMigrationComplete(vm) {
			state.IsComplete = true
			duration := time.Since(state.StartTime)
			slog.Info("Migration completed",
				"vm", vmName,
				"duration", duration,
				"vmop", state.VMOPName)

			// Clean up completed migration
			delete(m.migratingVMs, vmName)
		}
	}
}

// isMigrationComplete checks if a VM migration is complete
func (m *ContinuousMigrator) isMigrationComplete(vm *v1alpha2.VirtualMachine) bool {
	if vm.Status.Stats == nil || len(vm.Status.Stats.PhasesTransitions) < 2 {
		return false
	}

	transitions := vm.Status.Stats.PhasesTransitions
	last := transitions[len(transitions)-1]
	beforeLast := transitions[len(transitions)-2]

	// Migration is complete if we see Migrating -> Running transition
	return last.Phase == v1alpha2.MachineRunning && beforeLast.Phase == v1alpha2.MachineMigrating
}
