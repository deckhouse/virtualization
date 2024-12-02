package vd

import (
	"context"
	"fmt"
	"os"

	"statistic/internal/helpers"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	v1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VD struct {
	Name             string                                    `json:"name"`
	VirtualDiskStats v1alpha2.VirtualDiskStatsCreationDuration `json:"creationDuration,omitempty"`
}

type VDs struct {
	Items []VD `json:"items"`
}

func Get(client kubeclient.Client, namespace string) {
	var (
		vds                       VDs
		sumWaitingForDependencies float64
		sumDVCRProvisioning       float64
		sumTotalProvisioning      float64
	)

	// Limit & Continue for separete call res
	vdList, err := client.VirtualDisks(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to get vm: %v\n", err)
		os.Exit(1)
	}

	totalItems := len(vdList.Items)

	for _, vd := range vdList.Items {
		if string(vd.Status.Phase) == "Ready" {

			vds.Items = append(vds.Items, VD{
				Name: vd.Name,
				VirtualDiskStats: v1alpha2.VirtualDiskStatsCreationDuration{
					WaitingForDependencies: vd.Status.Stats.CreationDuration.WaitingForDependencies,
					DVCRProvisioning:       vd.Status.Stats.CreationDuration.DVCRProvisioning,
					TotalProvisioning:      vd.Status.Stats.CreationDuration.TotalProvisioning,
				},
			})

			sumWaitingForDependencies += helpers.ToSeconds(vd.Status.Stats.CreationDuration.WaitingForDependencies)
			sumDVCRProvisioning += helpers.ToSeconds(vd.Status.Stats.CreationDuration.DVCRProvisioning)
			sumTotalProvisioning += helpers.ToSeconds(vd.Status.Stats.CreationDuration.TotalProvisioning)
		}
	}

	avgWaitingForDependencies := sumWaitingForDependencies / float64(totalItems)
	avgDVCRProvisioning := sumDVCRProvisioning / float64(totalItems)
	avgTotalProvisioning := sumTotalProvisioning / float64(totalItems)

	fmt.Println("Total VDs count:", totalItems)

	fmt.Println("Average WaitingForDependencies in seconds:", avgWaitingForDependencies)
	fmt.Println("Average DVCRProvisioning in seconds:", avgDVCRProvisioning)
	fmt.Println("Average TotalProvisioning in seconds:", avgTotalProvisioning)
}
