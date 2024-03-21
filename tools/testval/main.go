package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

func getenv(key, def string) string {
	v := os.Getenv(key)
	if len(v) == 0 {
		return def
	}
	return v
}

func cmdGitDiff() ([]string, error) {
	githubHeadRef := fmt.Sprintf("origin/%s", getenv("GITHUB_HEAD_REF", "cicd/check-no_cyrillic"))

	gitCmd := exec.Command("git", "diff", "--name-only", "origin/main", githubHeadRef)
	outCmd, err := gitCmd.Output()
	if err != nil {
		return nil, err
	}
	outSrt := strings.TrimSuffix(string(outCmd), "\n")
	out := strings.Split(outSrt, "\n")

	return out, err
}

func noCyrilic(files []string) error {
	count := 0

	dirPath := "/Users/korolevn/repos/Virtualization-tasks/github/virtualization/"
	//path, err := os.Getwd()
	//if err != nil {
	//	log.Println(err)
	//}

	for _, fileName := range files {
		filePath := filepath.Join(dirPath, fileName)
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("Problem open file: %v\n", err)
			count += 1
			continue
		}
		scanner := bufio.NewScanner(file)

		for scanner.Scan() {
			line := scanner.Text()
			for i, char := range line {
				if unicode.Is(unicode.Cyrillic, char) {
					fmt.Printf("File %s with cyrillic char [%s] line [%d]\n", fileName, string(char), i)
					count += 1
					break
				}
			}
		}
		file.Close()
	}
	if count > 0 {
		return errors.New("Need to fix files")
	}
	fmt.Println("All clear")
	return nil
}

//git diff --name-only origin/main origin/${GITHUB_HEAD_REF} | grep -E '\.(go|md|yaml|yml)$'

func main() {
	res, err := cmdGitDiff()
	if err != nil {
		fmt.Println(err)
	}

	err = noCyrilic(res)
	if err != nil {
		fmt.Println(err)
	}
}
