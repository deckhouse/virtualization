package app

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewExportCommand() *cobra.Command {
	o := &exportOptions{}
	cmd := &cobra.Command{
		Use:     "export [:host:] [:busID:]",
		Short:   "Export USB device on USBIP server",
		Example: o.Usage(),
		RunE:    o.Run,
		Args:    cobra.ExactArgs(2),
	}

	o.AddFlags(cmd.Flags())

	return cmd
}

type exportOptions struct {
	port int
}

func (o *exportOptions) Usage() string {
	return `  # Export USB devices on USBIP server
  $ go-usbip export 192.168.1.1 3-1
`
}

func (o *exportOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&o.port, "port", 3240, "Remote port for exporting")
}

func (o *exportOptions) Run(_ *cobra.Command, args []string) error {
	host := args[0]
	busID := args[1]

	return usbip.NewUSBExporter().Export(host, busID, o.port)
}
