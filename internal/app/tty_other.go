//go:build !darwin && !linux

package app

import (
	"fmt"
	"os"
)

func terminalWidth(_ *os.File) (int, error) {
	return 0, fmt.Errorf("terminal width unsupported on this platform")
}
