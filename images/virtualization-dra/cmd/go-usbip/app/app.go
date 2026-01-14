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

package app

import "github.com/spf13/cobra"

const long = `
                             _     _
  __ _  ___        _   _ ___| |__ (_)_ __
 / _' |/ _ \ _____| | | / __| '_ \| | '_ \
| (_| | (_) |_____| |_| \__ \ |_) | | |_) |
\__, | \___/       \__,_|___/_.__/|_| .__/
|___/                             |_|

	go-usbip is a implementation of USBIP server and client.
`

func NewUSBIPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "usbip",
		Short:         "USBIP command line tool",
		Long:          long,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		NewRunCommand(),
		NewBindCommand(),
		NewUnbindCommand(),
		NewAttachCommand(),
		NewDetachCommand(),
		NewUsedPortsCommand(),
		NewUsedInfoCommand(),
	)

	return cmd
}
