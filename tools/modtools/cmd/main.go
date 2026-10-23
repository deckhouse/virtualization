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

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deckhouse/virtualization/tools/modtools/internal/command"
	"github.com/deckhouse/virtualization/tools/modtools/internal/report"
)

func main() {
	if err := command.NewCommand(filepath.Base(os.Args[0])).Execute(); err != nil {
		// The root command silences errors because a failed check has already
		// printed its own report and only returns ErrFailed as a signal.
		// Anything else (a bad flag value, for one) still has to be shown.
		if !errors.Is(err, report.ErrFailed) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
