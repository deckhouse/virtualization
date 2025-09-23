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

	"statistic/internal/helpers"
	"statistic/internal/vd"
	"statistic/internal/vm"

	"github.com/spf13/cobra"
)

var (
	namespace      string
	virtualmachine bool
	virtualdisk    bool
)

var rootCmd = &cobra.Command{
	Use:   "statistic",
	Short: "get statistic for vm and vd in name space",
	Long: `Get statistic from virtualmachine and virtualdisk form namespace, defaulr name space 'perf' and save to csv file

Example output for avg statistics:

Total VMs count: 30
Average WaitingForDependencies in seconds: 107.90
Average VirtualMachineStarting in seconds: 14.13
Average GuestOSAgentStarting in seconds: 145.43

csv files saved to current directory ./all-{vm/vd}-perf-2025-09-23_12-48-51.csv
`,
	Args: cobra.ArbitraryArgs,
	Run:  getStatistic,
}

func init() {
	rootCmd.Flags().StringVarP(&namespace, "ns", "n", "perf", "namespace to look for the VMs,VDs, default 'perf'")
	rootCmd.Flags().BoolVarP(&virtualmachine, "vm", "v", false, "get virtualmachine statistics")
	rootCmd.Flags().BoolVarP(&virtualdisk, "vd", "d", false, "get virtualdisk statistics")
}

func getStatistic(cmd *cobra.Command, args []string) {
	client := helpers.CreateKubeConfig()

	if virtualmachine {
		vm.GetStats(client, namespace)
	}

	if virtualdisk {
		vd.GetStats(client, namespace)
	}

	if !virtualmachine && !virtualdisk {
		vm.GetStats(client, namespace)
		vd.GetStats(client, namespace)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
