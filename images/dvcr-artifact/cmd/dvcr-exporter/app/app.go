package app

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/exporter"
)

func NewDVCRExporterCommand() *cobra.Command {
	opts := newOption()

	cmd := &cobra.Command{
		Use:           "dvcr-exporter",
		Short:         "DVCR exporter",
		Args:          cobra.NoArgs,
		RunE:          opts.Run,
		SilenceUsage:  false,
		SilenceErrors: false,
	}
	opts.AddFlags(cmd.Flags())
	return cmd
}

func newOption() *option {
	return &option{
		Config: &exporter.Config{},
	}
}

type option struct {
	Config *exporter.Config
}

func (o *option) AddFlags(fs *pflag.FlagSet) {
	o.Config.Load(fs)
}

func (o *option) Validate() error {
	return o.Config.Validate()
}

func (o *option) Run(cmd *cobra.Command, _ []string) error {
	err := o.Validate()
	if err != nil {
		return err
	}
	e, err := o.Config.Complete()
	if err != nil {
		return err
	}

	return e.Run(cmd.Context())
}
