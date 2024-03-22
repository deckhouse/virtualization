package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var skipDocRe = regexp.MustCompile(`doc-ru-.+\.y[a]?ml$|_RU\.md$|_ru\.html$|docs/site/_.+|docs/documentation/_.+`)

func getenv(key, def string) string {
	v := os.Getenv(key)
	if len(v) == 0 {
		return def
	}
	return v
}

func prepareListFiles(files []string) []string {
	var outListFiles []string

	for _, fileName := range files {
		if skipDocRe.MatchString(fileName) {
			continue
		}
		outListFiles = append(outListFiles, fileName)
	}
	fmt.Println(outListFiles, "<-- outListFiles")
	return outListFiles
}

func cmdGitDiffFilesList() ([]string, error) {
	githubHeadRef := fmt.Sprintf("origin/%s", getenv("GITHUB_HEAD_REF", "cicd/check-no_cyrillic"))

	gitCmd := exec.Command("git", "diff", "--name-only", "origin/main", githubHeadRef)
	outCmd, err := gitCmd.Output()
	if err != nil {
		return nil, err
	}
	outSrtTrim := strings.TrimSuffix(string(outCmd), "\n")
	outList := strings.Split(outSrtTrim, "\n")

	out := prepareListFiles(outList)

	return out, err
}

func noCyrilic(files []string) error {
	count := 0
	var lineNum int
	dirPath := "/Users/korolevn/repos/Virtualization-tasks/github/virtualization/"
	//dirPath, err := os.Getwd()
	//if err != nil {
	//	fmt.Println(dirPath)
	//	os.Exit(1)
	//}

	if len(files) == 0 {
		return nil
	}

	for _, fileName := range files {
		filePath := filepath.Join(dirPath, fileName)
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("Problem open file: %v\n", err)
			continue
		}
		scanner := bufio.NewScanner(file)
		lineNum = 0
		for scanner.Scan() {
			lineNum += 1
			line := scanner.Text()
			for _, char := range line {
				if unicode.Is(unicode.Cyrillic, char) {
					fmt.Printf("File %s with cyrillic char [%s] in line num [%d], line %s\n", fileName, string(char),
						lineNum, line)
					count += 1
					break
				}
			}
		}
		err = file.Close()
		if err != nil {
			panic(err)
		}
	}
	if count > 0 {
		return errors.New("Need to fix files")
	}
	fmt.Println("All clear")
	return nil
}

func main() {
	res, err := cmdGitDiffFilesList()
	if err != nil {
		fmt.Println(err)
	}

	err = noCyrilic(res)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
