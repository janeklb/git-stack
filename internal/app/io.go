package app

import "fmt"

func (a *App) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.stdout, format, args...)
}

func (a *App) print(args ...any) {
	_, _ = fmt.Fprint(a.stdout, args...)
}

func (a *App) println(args ...any) {
	_, _ = fmt.Fprintln(a.stdout, args...)
}
