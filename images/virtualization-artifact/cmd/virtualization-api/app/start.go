/*
Copyright 2024 Flant JSC

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
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/version"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/term"

	"github.com/deckhouse/virtualization-controller/cmd/virtualization-api/app/options"
)

func NewAPIServerCommand() *cobra.Command {
	opts := options.NewOptions()
	cmd := &cobra.Command{
		Short: "Launch virtualization-api server",
		Long:  "Launch virtualization-api server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := runCommand(c.Context(), opts); err != nil {
				return err
			}
			return nil
		},
	}
	fs := cmd.Flags()
	nfs := opts.Flags()
	for _, f := range nfs.FlagSets {
		fs.AddFlagSet(f)
	}
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	logs.AddGoFlags(local)
	nfs.FlagSet("logging").AddGoFlagSet(local)

	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		_, err := fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		if err != nil {
			return err
		}
		cliflag.PrintSections(cmd.OutOrStderr(), nfs, cols)
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		if err != nil {
			panic(err)
		}
		cliflag.PrintSections(cmd.OutOrStdout(), nfs, cols)
	})
	fs.AddGoFlagSet(local)
	return cmd
}

func runCommand(ctx context.Context, o *options.Options) error {
	if o.ShowVersion {
		fmt.Println(version.Get().GitVersion)
		return nil
	}

	err := o.Validate()
	if err != nil {
		return err
	}

	config, err := o.ServerConfig()
	if err != nil {
		return err
	}

	s, err := config.Complete()
	if err != nil {
		return err
	}

	return s.RunUntil(ctx)
}
