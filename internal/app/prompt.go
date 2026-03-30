package app

import (
	"bufio"
	"io"
	"strings"
)

func readPromptLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(line)), nil
}
