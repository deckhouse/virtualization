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

package main

import (
	"fmt"
	"os"

	"hooks/pkg/settings"

	"github.com/tidwall/gjson"
)

/*
Problems:

1. Current versions of deckhouse-controller (2025-08-08) and module-sdk (0.3.3)
are not reporting readiness probe errors in user accessible resources.
This hook is a simple workaround to see readiness problems in
module/virtualization status field.

Fix: No fixes yet.

2. Current version of module-sdk (0.3.3) not supports Queue for Schedule
configurations in batch hooks.

Fix: PR is merged, planned release version is 1.72, see https://github.com/deckhouse/deckhouse/pull/14961

Workaround:

Use classic shell hook to run schedules in a parallel queue.
Check value with error messsage:
If value is present, print it to stderr and exit with 1.
If no value, do nothing.

TODO remove this hook after fixes reached rock solid channel.
*/

func main() {
	// Print hook config to stdout if --config is passed.
	if len(os.Args) > 1 && os.Args[1] == "--config" {
		fmt.Printf(`{
  "configVersion": "v1",
  "schedule": [
  {
    "name": "report-module-config-validation-error",
    "crontab": "*/15 * * * * *",
    "queue": "/modules/virtualization/module-config-validation-error"
  }
  ]
}
`)
		os.Exit(0)
	}

	// No arguments -> run main hook handler.
	// Load values
	values, err := loadValues()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load values: %v", err)
		os.Exit(1)
	}

	err = handle(values)
	if err != nil {
		fmt.Fprintf(os.Stderr, "handle values: %v", err)
		os.Exit(1)
	}
}

func loadValues() (gjson.Result, error) {
	valuesPath := os.Getenv("VALUES_PATH")
	if valuesPath == "" {
		return gjson.Result{}, fmt.Errorf("wrong env: VALUES_PATH is empty, should be a path to values.json")
	}
	valuesBytes, err := os.ReadFile(valuesPath)
	if err != nil {
		return gjson.Result{}, err
	}
	if !gjson.ValidBytes(valuesBytes) {
		return gjson.Result{}, fmt.Errorf("invalid json in values file %s", valuesPath)
	}
	return gjson.ParseBytes(valuesBytes), nil
}

// handle returns error with message from
func handle(values gjson.Result) error {
	validationObj := values.Get(settings.InternalValuesConfigValidationPath)
	if validationObj.IsObject() {
		validationErr := validationObj.Get("error")
		if validationErr.Exists() {
			return fmt.Errorf(validationErr.String())
		}
	}
	return nil
}
