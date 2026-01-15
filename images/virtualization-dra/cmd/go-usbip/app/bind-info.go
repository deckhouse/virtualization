package app

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

func NewBindInfoCommand() *cobra.Command {
	o := &bindInfoOptions{}
	cmd := &cobra.Command{
		Use:     "bind-info",
		Short:   "Get bind info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type bindInfoOptions struct{}

func (o *bindInfoOptions) Usage() string {
	return `  # Get bind info
  $ go-usbip bind-info
`
}

func (o *bindInfoOptions) Run(cmd *cobra.Command, _ []string) error {
	infos, err := usbip.NewUSBBinder().GetBindInfo()
	if err != nil {
		return err
	}

	bytes, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	cmd.Println(string(bytes))

	return nil
}
