package app

import (
	"fmt"
	"io"
	"os"
)

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.stdout, format, args...)
}

func (a *App) printlnf(format string, args ...any) {
	fmt.Fprintf(a.stdout, format+"\n", args...)
}

func (a *App) print(args ...any) {
	fmt.Fprint(a.stdout, args...)
}

func (a *App) println(args ...any) {
	fmt.Fprintln(a.stdout, args...)
}

func (a *App) printferr(format string, args ...any) {
	fmt.Fprintf(a.stderr, format, args...)
}

func (a *App) printferrln(format string, args ...any) {
	fmt.Fprintf(a.stderr, format+"\n", args...)
}

func stdoutIsTTY(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
