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

2. Current version of module-sdk (0.3.3) not supports Queue for Schedule
configurations in batch hooks.

Workaround:

Use classic shell hook to run schedules in a parallel queue.
Check value with error messsage:
If value is present, print it to stderr and exit with 1.
If no value, do nothing.

TODO remove this hook after implementing readiness probe error reporting in the deckhouse-controller.
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

func handle(values gjson.Result) error {
	readinessObj := values.Get(settings.InternalValuesConfigValidationPath)
	if !readinessObj.IsObject() {
		return fmt.Errorf("module is not ready yet")
	}
	validationErr := readinessObj.Get("moduleConfigValidationError")
	if validationErr.Exists() {
		return fmt.Errorf(validationErr.String())
	}
	return nil
}
