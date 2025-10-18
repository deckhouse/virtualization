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

package vmop

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"statistic/internal/helpers"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VMOP struct {
	Name      string        `json:"name"`
	Phase     string        `json:"phase"`
	Duration  time.Duration `json:"duration"`
	StartTime metav1.Time   `json:"startTime"`
	EndTime   metav1.Time   `json:"endTime"`
}

type VMOPs struct {
	Items []VMOP `json:"items"`
}

func (vmops *VMOPs) SaveToCSV(ns string, outputDir string) {
	filepath := fmt.Sprintf("/all-%s-%s-%s.csv", "vmop", ns, time.Now().Format("2006-01-02_15-04-05"))

	file, err := os.Create(outputDir + filepath)
	if err != nil {
		os.Exit(1)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"Name", "Phase", "Duration", "StartTime", "EndTime"}
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing header to CSV file: %v\n", err)
		os.Exit(1)
	}

	for _, res := range vmops.Items {
		data := []string{
			res.Name,
			res.Phase,
			helpers.DurationToString(&metav1.Duration{Duration: res.Duration}),
			res.StartTime.Format(time.RFC3339),
			res.EndTime.Format(time.RFC3339),
		}
		if err := writer.Write(data); err != nil {
			fmt.Printf("Error writing data to CSV file: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Data of VMOP saved successfully to csv", file.Name())
}

func GetStatistic(client kubeclient.Client, namespace string, outputDir string) {
	vmopList, err := client.VirtualMachineOperations(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to get vmop: %v\n", err)
		os.Exit(1)
	}

	var (
		vmops       VMOPs
		sumDuration float64
	)

	totalItems := len(vmopList.Items)
	processedCount := 0

	for _, vmop := range vmopList.Items {
		// Calculate duration from start to end
		var duration time.Duration
		var startTime, endTime metav1.Time

		// Find start and end times from conditions
		if len(vmop.Status.Conditions) > 0 {
			// Sort conditions by LastTransitionTime to find earliest and latest
			conditions := vmop.Status.Conditions

			// Find the earliest (start) time
			startTime = conditions[0].LastTransitionTime
			for _, condition := range conditions {
				if condition.LastTransitionTime.Time.Before(startTime.Time) {
					startTime = condition.LastTransitionTime
				}
			}

			// Find the latest (end) time
			endTime = conditions[0].LastTransitionTime
			for _, condition := range conditions {
				if condition.LastTransitionTime.Time.After(endTime.Time) {
					endTime = condition.LastTransitionTime
				}
			}
		}

		// Only calculate duration if we have valid start and end times
		if !endTime.IsZero() && !startTime.IsZero() && endTime.Time.After(startTime.Time) {
			duration = endTime.Time.Sub(startTime.Time)
		} else {
			// If we can't determine duration, set to 0
			duration = 0
		}

		vmops.Items = append(vmops.Items, VMOP{
			Name:      vmop.Name,
			Phase:     string(vmop.Status.Phase),
			Duration:  duration,
			StartTime: startTime,
			EndTime:   endTime,
		})

		// Only add to sum if duration is positive
		if duration.Seconds() > 0 {
			sumDuration += duration.Seconds()
		}
		processedCount++
	}

	// Calculate average duration (only for VMOPs with positive duration)
	avgDuration := float64(0)
	validDurationCount := 0
	for _, vmop := range vmops.Items {
		if vmop.Duration.Seconds() > 0 {
			validDurationCount++
		}
	}

	if validDurationCount > 0 {
		avgDuration = sumDuration / float64(validDurationCount)
	}

	saveData := fmt.Sprintf(
		"Total VMOPs count: %d\n"+
			"Processed VMOPs count: %d\n"+
			"Valid Duration VMOPs: %d\n"+
			"Average Duration in seconds: %.2f\n",
		totalItems, processedCount, validDurationCount, avgDuration,
	)

	helpers.SaveToFile(saveData, "avg-vmop", namespace, outputDir)

	fmt.Println(saveData)

	vmops.SaveToCSV(namespace, outputDir)
}
