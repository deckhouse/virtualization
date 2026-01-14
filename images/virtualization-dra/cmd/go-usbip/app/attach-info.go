package app

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization-dra/internal/usbip"
)

func NewAttachInfoCommand() *cobra.Command {
	o := &attachInfoOptions{}
	cmd := &cobra.Command{
		Use:     "attach-info",
		Short:   "Get attach info",
		Example: o.Usage(),
		RunE:    o.Run,
	}

	return cmd
}

type attachInfoOptions struct{}

func (o *attachInfoOptions) Usage() string {
	return `  # Get attach info
  $ go-usbip attach-info
`
}

func (o *attachInfoOptions) Run(cmd *cobra.Command, _ []string) error {
	infos, err := usbip.NewUSBAttacher().GetAttachInfo()
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
