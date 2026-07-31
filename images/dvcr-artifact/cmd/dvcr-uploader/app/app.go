/*
Copyright 2026 Flant JSC

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

package app

import (
	goflag "flag"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/uploader"
)

// NewUploaderCommand builds the root command for the dvcr-uploader binary.
func NewUploaderCommand() *cobra.Command {
	o := &uploader.Options{}

	var verbosity int

	cmd := &cobra.Command{
		Use:           "uploader",
		Short:         "dvcr-uploader receives an uploaded image and pushes it to the DVCR registry",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return setupKlogVerbosity(verbosity)
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			server, err := o.Complete()
			if err != nil {
				return err
			}

			klog.Infof("Starting uploader on %s:%d", o.ListenAddress, o.ListenPort)

			return server.Run()
		},
	}

	o.AddFlags(cmd.Flags())
	cmd.Flags().IntVarP(&verbosity, "v", "v", 0, "Number for the log level verbosity")

	return cmd
}

func setupKlogVerbosity(verbosity int) error {
	fs := goflag.NewFlagSet("klog", goflag.ContinueOnError)
	klog.InitFlags(fs)
	if err := fs.Set("v", strconv.Itoa(verbosity)); err != nil {
		return fmt.Errorf("set klog verbosity: %w", err)
	}
	return nil
}
