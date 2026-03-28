package main

import (
	"os"
	"path/filepath"

	"github.com/janeklb/stack/internal/app"
)

func main() {
	cli := app.New()
	os.Exit(cli.Run(os.Args[1:], filepath.Base(os.Args[0])))
}
