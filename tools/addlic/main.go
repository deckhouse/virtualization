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

		if fileToCheckRe.MatchString(filePath) && !fileToSkipRe.MatchString(filePath) {
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
