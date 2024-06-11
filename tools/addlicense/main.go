package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var licenseText = fmt.Sprintf(`Copyright %d Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
`, time.Now().Year())

var goLicense = "/*" + licenseText + " */\n\n"
var bashLicense = "\n# " + strings.ReplaceAll(strings.TrimSpace(licenseText), "\n", "\n# ") + "\n\n"
var fileToCheckRe = regexp.MustCompile(`\.go$|/[^/.]+$|\.sh$|\.py$|^\.github/(scripts|workflows|workflow_templates)/.+\.(js|yml|yaml|sh)$`)
var fileToSkipRe = regexp.MustCompile(`geohash.lua$|\.
github/CODEOWNERS|Dockerfile$|Makefile$|Taskfile|/docs/|bashrc$|inputrc$|modules_menu_skip$
|LICENSE$`)

var CELicenseRe = regexp.MustCompile(`(?s)[/#{!-]*(\s)*Copyright 20[2-9][1-9] Flant JSC[-!}\s\n#/]*
[/#{!-]*(\s)*Licensed under the Apache License, Version 2.0 \(the "License"\);[-!}\n]*
[/#{!-]*(\s)*you may not use this file except in compliance with the License.[-!}\n]*
[/#{!-]*(\s)*You may obtain a copy of the License at[-!}\n\s#/]*
[/#{!-]*(\s)*http://www.apache.org/licenses/LICENSE-2.0[-!}\n\s#/]*
[/#{!-]*(\s)*Unless required by applicable law or agreed to in writing, software[-!}\n]*
[/#{!-]*(\s)*distributed under the License is distributed on an "AS IS" BASIS,[-!}\n]*
[/#{!-]*(\s)*WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.[-!}\n]*
[/#{!-]*(\s)*See the License for the specific language governing permissions and[-!}\n]*
[/#{!-]*(\s)*limitations under the License.[-!}\n]{0,1}`)

func isShebangLine(line string) bool {
	return strings.HasPrefix(line, "#!")
}

func closeFile(f *os.File) {
	err := f.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func addLicenseToFile(filePath, license string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer closeFile(f)

	reader := bufio.NewReader(f)
	firstLine, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("failed to read first line of file: %w", err)
	}

	restOfFile, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read rest of file: %w", err)
	}

	fullContent := append([]byte(firstLine), restOfFile...)
	if CELicenseRe.MatchString(string(fullContent)) {
		//if strings.Contains(string(fullContent), license) {
		fmt.Printf("File %s already contains the license. Skipping.\n", filePath)
		return nil
	} else {
		fmt.Printf("Add lic %s\n", filePath)
	}

	var newContent []byte
	if isShebangLine(firstLine) {
		newContent = append([]byte(firstLine), append([]byte(license), restOfFile...)...)
	} else {
		newContent = append([]byte(license), append([]byte(firstLine), restOfFile...)...)
	}

	return os.WriteFile(filePath, newContent, 0644)
}

func getLicenseForFile(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return goLicense
	case ".sh":
		fallthrough
	case ".bash":
		fallthrough
	case ".zsh":
		return bashLicense
	default:
		return ""
	}
}

func main() {

	//directory := "./example"
	var directory string

	dirArg := flag.String("directory", "", "The directory containing the files")
	flag.Parse()

	// Ensure the directory is provided
	if *dirArg == "" {
		fmt.Println("No directory provided. Use the -directory flag to specify the directory.")
		//directory = "./example/"
		return
	} else {
		fmt.Println("Directory provided:", *dirArg)
	}

	directory, err := filepath.Abs(*dirArg)
	if err != nil {
		fmt.Println("Cannot get absolute path of directory:", err)
	}
	err = filepath.Walk(directory, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if fileToCheckRe.MatchString(filePath) && !fileToSkipRe.MatchString(filePath) {
			license := getLicenseForFile(filePath)
			if license == "" {
				fmt.Printf("Skipping file %s: Unsupported file extension.\n", info.Name())
			}

			err = addLicenseToFile(filePath, license)
			if err != nil {
				fmt.Println(err)
			}
		} else {
			fmt.Printf("Skipping file: %s\n", filePath)
		}

		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}
