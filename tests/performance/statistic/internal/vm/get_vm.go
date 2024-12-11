package vm

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"time"

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

func (vms *VMs) SaveToCSV(ns string) {
	filepath := fmt.Sprintf("/all-%s-%s-%s.csv", "vm", ns, time.Now().Format("2006-01-02_15-04-05"))
	execpath, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}

	file, err := os.Create(execpath + filepath)
	if err != nil {
		os.Exit(1)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Name", "WaitingForDependencies", "VirtualMachineStarting", "GuestOSAgentStarting"}
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing header to CSV file: %v\n", err)
		os.Exit(1)
	}

	for _, res := range vms.Items {

		data := []string{
			res.Name,
			helpers.DurationToString(res.VirtualMachineLaunchTimeDuration.WaitingForDependencies),
			helpers.DurationToString(res.VirtualMachineLaunchTimeDuration.VirtualMachineStarting),
			helpers.DurationToString(res.VirtualMachineLaunchTimeDuration.GuestOSAgentStarting),
		}
		if err := writer.Write(data); err != nil {
			fmt.Printf("Error writing data to CSV file: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Data of VD saved successfully to csv", file.Name())
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
				// Name: vm.Status.Stats.PhasesTransitions[len(vm.Status.Stats.PhasesTransitions)-1], use range
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

	saveData := fmt.Sprintf(
		"Total VMs count: %d\n"+
			"Average WaitingForDependencies in seconds: %.2f\n"+
			"Average VirtualMachineStarting in seconds: %.2f\n"+
			"Average GuestOSAgentStarting in seconds: %.2f\n",
		totalItems, avgWaitingForDependencies, avgVirtualMachineStarting, avgGuestOSAgentStarting,
	)

	helpers.SaveToFile(saveData, "vm", namespace)

	fmt.Println("Total VMs:", totalItems)

	fmt.Println("Average WaitingForDependencies in seconds:", avgWaitingForDependencies)
	fmt.Println("Average VirtualMachineStarting in seconds:", avgVirtualMachineStarting)
	fmt.Println("Average GuestOSAgentStarting in seconds:", avgGuestOSAgentStarting)

	vms.SaveToCSV(namespace)
}
