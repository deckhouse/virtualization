package lifecycle

import (
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/deckhouse-cli/internal/virtualization/templates"
)

func NewEvictCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	lifecycle := NewLifecycle(Evict, clientConfig)
	cmd := &cobra.Command{
		Use:     "evict (VirtualMachine)",
		Short:   "Evict a virtual machine.",
		Example: lifecycle.Usage(),
		Args:    templates.ExactArgs("evict", 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lifecycle.Run(args)
		},
	}
	AddCommandlineArgs(cmd.Flags(), &lifecycle.opts)
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}
