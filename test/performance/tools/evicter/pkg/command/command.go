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

package command

import (
	"fmt"
	"os"
	"time"

	"evicter/helpers"
	"evicter/internal"

	"github.com/spf13/cobra"
)

var (
	namespace string
	target    int
	duration  string
)

var rootCmd = &cobra.Command{
	Use:   "migrator",
	Short: "continuously migrate a percentage of virtual machines",
	Long:  `A tool that continuously migrates a specified percentage of virtual machines in a namespace`,
	Args:  cobra.ArbitraryArgs,
	Run:   startMigrator,
}

func init() {
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "perf", "namespace to look for the VMs, default 'perf'")
	rootCmd.Flags().IntVarP(&target, "target", "t", 10, "target percentage for VM migration (1-100)")
	rootCmd.Flags().StringVarP(&duration, "duration", "d", "0", "duration to run the migrator (e.g., '30m', '1h', '0' for infinite). Default '0' for infinite")
}

func startMigrator(cmd *cobra.Command, args []string) {
	// Validate target percentage
	if target < 1 || target > 100 {
		fmt.Println("Error: target percentage must be between 1 and 100")
		os.Exit(1)
	}

	// Parse duration
	var runDuration time.Duration
	var err error
	if duration != "0" {
		runDuration, err = time.ParseDuration(duration)
		if err != nil {
			fmt.Printf("Error parsing duration '%s': %v\n", duration, err)
			os.Exit(1)
		}
	}

	// create client
	client := helpers.CreateKubeConfig()

	// Start the continuous migrator
	internal.StartContinuousMigrator(client, namespace, target, runDuration)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
