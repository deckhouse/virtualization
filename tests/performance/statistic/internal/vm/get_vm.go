package vm

import (
	"context"
	"fmt"
	"os"

	"statistic/internal/helpers"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VM struct {
	Name                             string                                    `json:"name"`
	VirtualMachineLaunchTimeDuration v1alpha2.VirtualMachineLaunchTimeDuration `json:"launchTimeDuration"`
}

type VMs struct {
	Items []VM `json:"items"`
}

func Get(client kubeclient.Client, namespace string) {
	vmList, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to get vm: %v\n", err)
		os.Exit(1)
	}

	var (
		vms                       VMs
		sumWaitingForDependencies float64
		sumVirtualMachineStarting float64
		sumGuestOSAgentStarting   float64
	)

	totalItems := len(vmList.Items)

	for _, vm := range vmList.Items {
		if string(vm.Status.Phase) == "Running" {

			vms.Items = append(vms.Items, VM{
				Name: vm.Name,
				VirtualMachineLaunchTimeDuration: v1alpha2.VirtualMachineLaunchTimeDuration{
					WaitingForDependencies: vm.Status.Stats.LaunchTimeDuration.WaitingForDependencies,
					VirtualMachineStarting: vm.Status.Stats.LaunchTimeDuration.VirtualMachineStarting,
					GuestOSAgentStarting:   vm.Status.Stats.LaunchTimeDuration.GuestOSAgentStarting,
				},
			})

			sumWaitingForDependencies += helpers.ToSeconds(vm.Status.Stats.LaunchTimeDuration.WaitingForDependencies)
			sumVirtualMachineStarting += helpers.ToSeconds(vm.Status.Stats.LaunchTimeDuration.VirtualMachineStarting)
			sumGuestOSAgentStarting += helpers.ToSeconds(vm.Status.Stats.LaunchTimeDuration.GuestOSAgentStarting)
		}
	}

	avgWaitingForDependencies := sumWaitingForDependencies / float64(totalItems)
	avgVirtualMachineStarting := sumVirtualMachineStarting / float64(totalItems)
	avgGuestOSAgentStarting := sumGuestOSAgentStarting / float64(totalItems)

	fmt.Println("Total VMs:", totalItems)

	fmt.Println("Average WaitingForDependencies in seconds:", avgWaitingForDependencies)
	fmt.Println("Average VirtualMachineStarting in seconds:", avgVirtualMachineStarting)
	fmt.Println("Average GuestOSAgentStarting in seconds:", avgGuestOSAgentStarting)
}
