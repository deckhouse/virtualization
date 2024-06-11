package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

func getLicenseForFile(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return goLicense
	case ".sh":
		fallthrough
	case ".py":
		fallthrough
	case ".bash":
		fallthrough
	case ".zsh":
		return bashPythonLicense
	default:
		return ""
	}
}

func checkLicense(fullContent []byte, filePath string) (Message, bool) {
	if CELicenseRe.MatchString(string(fullContent)) {
		msg := NewSkip(filePath, "File already contains CE license")
		return msg, true
	}

	if copyrightOrAutogenRe.MatchString(string(fullContent)) {
		msg := NewSkip(filePath, "File already contains autogenerated license")
		return msg, true
	}

	if flantRe.MatchString(string(fullContent)) {
		msg := NewSkip(filePath, "File already contains Flant license")
		return msg, true
	}

	if copyrightRe.MatchString(string(fullContent)) {
		msg := NewSkip(filePath, "File contains other license")
		return msg, true
	}

	return Message{}, false
}

func addLicenseToFile(filePath, license string) Message {
	f, err := os.Open(filePath)
	if err != nil {
		message := NewError(filepath.Base(filePath), "Error opening file", err.Error())
		return message
	}
	defer closeFile(f)

	reader := bufio.NewReader(f)
	firstLine, err := reader.ReadString('\n')
	if err != nil && err.Error() != "EOF" {
		message := NewError(filepath.Base(filePath), "Failed to read first line", err.Error())
		return message
	}

	restOfFile, err := io.ReadAll(reader)
	if err != nil {
		message := NewError(filepath.Base(filePath), "Failed to read rest of file", err.Error())
		return message
	}

	fullContent := append([]byte(firstLine), restOfFile...)

	message, lic := checkLicense(fullContent, filePath)
	var newContent []byte

	if !lic {
		if isShebangLine(firstLine) {
			newContent = append([]byte(firstLine), append([]byte(license), restOfFile...)...)

		} else {
			newContent = append([]byte(license), append([]byte(firstLine), restOfFile...)...)
		}
	} else {
		return message
	}

	err = os.WriteFile(filePath, newContent, 0644)

	if err != nil {
		message = NewError(filepath.Base(filePath), "Failed to write file", err.Error())
		return message
	}

	message = NewAdd(filepath.Base(filePath))
	return message
	//return os.WriteFile(filePath, newContent, 0644)
}
