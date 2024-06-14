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
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var directory string
	msgs := NewMessages()

	dirArg := flag.String("directory", "", "The directory containing the files")
	flag.Parse()

	if *dirArg == "" {
		fmt.Println("No directory provided. Use the -directory flag to specify the directory.")
		return
	} else {
		fmt.Println("Directory provided:", *dirArg)
	}

	directory, err := filepath.Abs(*dirArg)
	if err != nil {
		fmt.Println("Cannot get absolute path of directory:", err)
		return
	}

	err = filepath.Walk(directory, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if fileToSkipRe.MatchString(filePath) {
			return nil
		}

		if fileToCheckRe.MatchString(filePath) {
			lic := getLicenseForFile(filePath)
			msg := addLicenseToFile(filePath, lic)
			msgs.Add(msg)
		}

		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
	msgs.PrintReport()
	fmt.Println("script done")
}
