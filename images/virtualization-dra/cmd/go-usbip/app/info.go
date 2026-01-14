package app

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

func NewUsedInfoCommand() *cobra.Command {
	o := &usedInfoOptions{}
	cmd := &cobra.Command{
		Use:     "info",
		Short:   "Get used info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type usedInfoOptions struct{}

func (o *usedInfoOptions) Usage() string {
	return `  # Get used info
  $ go-usbip info
`
}

func (o *usedInfoOptions) Run(cmd *cobra.Command, _ []string) error {
	infos, err := usbip.NewUSBAttacher().GetUsedInfo()
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
