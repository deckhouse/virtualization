package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const licenseText = `#
# THIS FILE IS GENERATED, PLEASE DO NOT EDIT.
#

# Copyright 2023 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

`

var fileToCheckRe = regexp.MustCompile(`\.go$|/[^/.]+$|\.sh$|\.lua$|\.py$|^\.github/(scripts|workflows|workflow_templates)/.+\.(js|yml|yaml|sh)$`)
var fileToSkipRe = regexp.MustCompile(`geohash.lua$|\.
github/CODEOWNERS|Dockerfile$|Makefile$|Taskfile|/docs/|bashrc$|inputrc$|modules_menu_skip$
|LICENSE$`)

func addLic(path string) error {
	//filePath := filepath.Join(directory, file.Name())

	// Read the existing content of the file
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("Failed to read file %s: %v\n", path, err)
		return err
	}

	// Check if the file already contains the license
	if strings.Contains(string(content), licenseText) {
		fmt.Printf("File %s already contains the license. Skipping.\n", path)
		return nil
	}

	// Prepend the license text to the existing content
	newContent := append([]byte(licenseText), content...)

	// Write the new content back to the file
	err = os.WriteFile(path, newContent, 0644)
	if err != nil {
		fmt.Printf("Failed to write file %s: %v\n", path, err)
		return err
	} else {
		fmt.Printf("License added to file %s.\n", path)
		return nil
	}
	return nil
}

func main() {

	// Define the directory containing the files
	directory := "../../tests/"
	//directory := "./example"

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fileToCheckRe.MatchString(path) && !fileToSkipRe.MatchString(path) {
			fmt.Printf("Skipping %s\n", path)
		} else {
			fmt.Printf("Adding %s\n", path)
		}

		fmt.Println(path, info.Size(), info.Name())
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
	// Read all files in the directory
	//files, err := os.ReadDir(directory)
	//if err != nil {
	//	fmt.Printf("Failed to read directory: %v\n", err)
	//	return
	//}
	//
	//for _, file := range files {
	//	// We only care about regular files, not directories
	//	if file.IsDir() {
	//		continue
	//	}
	//
	//	// Get the full file path
	//	filePath := filepath.Join(directory, file.Name())
	//
	//	// Read the existing content of the file
	//	content, err := os.ReadFile(filePath)
	//	if err != nil {
	//		fmt.Printf("Failed to read file %s: %v\n", file.Name(), err)
	//		continue
	//	}
	//
	//	// Check if the file already contains the license
	//	if strings.Contains(string(content), licenseText) {
	//		fmt.Printf("File %s already contains the license. Skipping.\n", file.Name())
	//		continue
	//	}
	//
	//	// Prepend the license text to the existing content
	//	newContent := append([]byte(licenseText), content...)
	//
	//	// Write the new content back to the file
	//	err = os.WriteFile(filePath, newContent, 0644)
	//	if err != nil {
	//		fmt.Printf("Failed to write file %s: %v\n", file.Name(), err)
	//	} else {
	//		fmt.Printf("License added to file %s.\n", file.Name())
	//	}
	//}
}
