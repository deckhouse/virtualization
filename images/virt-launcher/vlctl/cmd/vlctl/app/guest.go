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

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewGuestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guest",
		Short: "Guest commands",
	}

	cmd.AddCommand(
		NewGuestInfoCommand(),
		NewGuestUsersCommand(),
		NewGuestFilesystemsCommand(),
		NewGuestPingCommand(),
	)

	return cmd
}

func NewGuestInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Get general guest info",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runGuestInfoCommand(baseOpts)
		},
	}
}

func runGuestInfoCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	info, err := client.GetGuestInfo()
	if err != nil {
		return fmt.Errorf("failed to get guest info: %w", err)
	}

	return marshalAndPrintOutput(&opts, info)
}

func NewGuestUsersCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "users",
		Short: "Get info about logged-in guest users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runGuestUsersCommand(baseOpts)
		},
	}
}

func runGuestUsersCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	users, err := client.GetUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	return marshalAndPrintOutput(&opts, users.Items)
}

func NewGuestFilesystemsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "filesystems",
		Short: "Get info about the guest's filesystems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runGuestFilesystemsCommand(baseOpts)
		},
	}
}

func runGuestFilesystemsCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	filesystems, err := client.GetFilesystems()
	if err != nil {
		return fmt.Errorf("failed to get filesystems: %w", err)
	}

	return marshalAndPrintOutput(&opts, filesystems.Items)
}

func NewGuestPingCommand() *cobra.Command {
	var timeout int32

	cmd := &cobra.Command{
		Use:   "ping [domainname]",
		Short: "Ping guest agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runGuestPingCommand(baseOpts, timeout)
		},
	}

	cmd.Flags().Int32VarP(&timeout, "timeout", "t", 30, "Timeout in seconds")

	return cmd
}

func runGuestPingCommand(opts BaseOptions, timeout int32) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	domain, exist, err := client.GetDomain()
	if err != nil {
		return fmt.Errorf("failed to get domain: %w", err)
	}
	if !exist {
		return fmt.Errorf("domain does not exist")
	}

	err = client.GuestPing(domain.Spec.Name, timeout)
	if err != nil {
		return fmt.Errorf("failed to ping: %w", err)
	}

	_, err = fmt.Fprintln(os.Stdout, "PONG")
	return err
}
