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

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var GcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage collector",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var gcRunCmd = &cobra.Command{
	Use:   "run",
	Short: "run `garbage-collect` deletes cache data and layers not referenced by any manifests",
	Args:  cobra.OnlyValidArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := Confirm()
		if errors.Is(err, promptui.ErrAbort) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("confirm is failed: %w", err)
		}

		vdCachePath := fmt.Sprintf("%s/vd", ImageRepoDir)
		err = os.RemoveAll(vdCachePath)
		if err != nil {
			return fmt.Errorf("cache data cannot be deleted: %w", err)
		}

		execCmd := exec.Command("registry", "garbage-collect", "/etc/docker/registry/config.yml", "--delete-untagged")
		stdout, err := execCmd.Output()
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}

		fmt.Println(string(stdout))
		return nil
	},
}

func Confirm() error {
	prompt := promptui.Prompt{
		Label:     "Confirm",
		IsConfirm: true,
	}

	_, err := prompt.Run()
	if err != nil {
		return err
	}

	return nil
}

func init() {
	GcCmd.AddCommand(gcRunCmd)
}
