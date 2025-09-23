package command

import (
	"fmt"
	"os"

	"evicter/helpers"
	"evicter/internal"

	"github.com/spf13/cobra"
)

var (
	namespace string
	target    int
)

var rootCmd = &cobra.Command{
	Use:   "statistic",
	Short: "get statistic for vm and vd in name space",
	Long:  `large describe`,
	Args:  cobra.ArbitraryArgs,
	Run:   startEvict,
}

func init() {
	rootCmd.Flags().StringVarP(&namespace, "ns", "n", "perf", "namespace to look for the VMs,VDs, default 'perf'")
	rootCmd.Flags().IntVarP(&target, "target", "t", 10, "target percentage for VM evict")
}

func startEvict(cmd *cobra.Command, args []string) {
	// create client
	client := helpers.CreateKubeConfig()

	vms := internal.ShuffleVMs(client, namespace)
	internal.VmopMigrateVMs(client, namespace, vms)
	internal.MonMigr(client, namespace, vms)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
