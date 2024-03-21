package val

import (
	"bufio"
	"fmt"
	"os"
	"unicode"
)

func main() {
	// Укажите путь к файлу, который нужно проверить
	filePath := "example.txt"

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Ошибка открытия файла:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		for _, char := range line {
			if unicode.Is(unicode.Cyrillic, char) {
				fmt.Println("Файл содержит кириллические символы")
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Ошибка при сканировании файла:", err)
		return
	}

	fmt.Println("Файл не содержит кириллических символов")
}
