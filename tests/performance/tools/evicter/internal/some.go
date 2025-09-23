package internal

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Count of vm that should migrated
func countEvictVMs(target int, vmListTotal []v1alpha2.VirtualMachine) int {
	vmNumTargetInMigration := target * len(vmListTotal) / 100
	return vmNumTargetInMigration
}

// Create map from shuffled VMs slice
func ShuffleVMs(client kubeclient.Client, namespace string) map[string]v1alpha2.VirtualMachine {
	source := rand.NewSource(time.Now().UnixNano())
	shuffle := rand.New(source)
	mapVMs := make(map[string]v1alpha2.VirtualMachine)

	listVMs, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		slog.Error("ShuffleVMs ", "err:", err)
		os.Exit(1)
	}

	vms := listVMs.Items
	shuffle.Shuffle(len(vms), func(i, j int) {
		vms[i], vms[j] = vms[j], vms[i]
	})

	countVMs := countEvictVMs(10, vms)
	for _, vm := range vms[:countVMs] {
		mapVMs[vm.Name] = vm
	}
	slog.Info("Shuffled VMs: ", "count:", len(mapVMs))

	return mapVMs
}

func newVMOP(vmName, namespace string, t v1alpha2.VMOPType) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.VirtualMachineOperationKind,
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: vmName + "-",
			Namespace:    namespace,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           t,
			VirtualMachine: vmName,
			Force:          false,
		},
	}
}

func VmopMigrateVMs(client kubeclient.Client, namespace string, vms map[string]v1alpha2.VirtualMachine) {
	ctx := context.TODO()
	t := v1alpha2.VMOPTypeEvict
	opts := metav1.CreateOptions{}

	for _, vm := range vms {
		slog.Info("VM to evict ", "name:", vm.Name)

		newVmop := newVMOP(vm.Name, namespace, t)
		vmop, err := client.VirtualMachineOperations(namespace).Create(ctx, newVmop, opts)
		if err != nil {
			slog.Error("VMOP create error: ", "name:", vmop.Name, "err:", err)
			continue
		}
		slog.Info("VMOP created: ", "name:", vmop.Name)
	}
}

func MonMigr(client kubeclient.Client, namespace string, vms map[string]v1alpha2.VirtualMachine) {
	ctx := context.TODO()

	for len(vms) > 0 {
		for _, vm := range vms {
			v, err := client.VirtualMachines(namespace).Get(ctx, vm.Name, metav1.GetOptions{})
			if err != nil {
				slog.Error("MonMigr", "error: ", err)
				os.Exit(1)
			}

			last := v.Status.Stats.PhasesTransitions[len(v.Status.Stats.PhasesTransitions)-1]
			beforeLast := v.Status.Stats.PhasesTransitions[len(v.Status.Stats.PhasesTransitions)-2]

			if last.Phase == v1alpha2.MachineMigrating && beforeLast.Phase == v1alpha2.MachineRunning {
				delete(vms, vm.Name)
			}
		}
		time.Sleep(1 * time.Second)
		slog.Info("VMs left: ", "count:", len(vms))
	}
}
