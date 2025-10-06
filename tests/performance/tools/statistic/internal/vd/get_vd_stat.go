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

package vd

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"statistic/internal/helpers"
	"time"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VD struct {
	Name             string                                    `json:"name"`
	VirtualDiskStats v1alpha2.VirtualDiskStatsCreationDuration `json:"creationDuration,omitempty"`
}

type VDs struct {
	Items []VD `json:"items"`
}

func (vds *VDs) SaveToCSV(ns string) {
	filepath := fmt.Sprintf("/all-%s-%s-%s.csv", "vd", ns, time.Now().Format("2006-01-02_15-04-05"))
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

	header := []string{"Name", "WaitingForDependencies", "DVCRProvisioning", "TotalProvisioning"}
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing header to CSV file: %v\n", err)
		os.Exit(1)
	}

	for _, res := range vds.Items {

		data := []string{
			res.Name,
			helpers.DurationToString(res.VirtualDiskStats.WaitingForDependencies),
			helpers.DurationToString(res.VirtualDiskStats.DVCRProvisioning),
			helpers.DurationToString(res.VirtualDiskStats.TotalProvisioning),
		}
		if err := writer.Write(data); err != nil {
			fmt.Printf("Error writing data to CSV file: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Data of VD saved successfully to csv", file.Name())
}

func GetStatistic(client kubeclient.Client, namespace string) {
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

	saveData := fmt.Sprintf(
		"Total VDs count: %d\n"+
			"Average WaitingForDependencies in seconds: %.2f\n"+
			"Average DVCRProvisioning in seconds: %.2f\n"+
			"Average TotalProvisioning in seconds: %.2f\n",
		totalItems, avgWaitingForDependencies, avgDVCRProvisioning, avgTotalProvisioning,
	)

	helpers.SaveToFile(saveData, "vd", namespace)

	fmt.Println(saveData)

	vds.SaveToCSV(namespace)
}
