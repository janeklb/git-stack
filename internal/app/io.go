package app

import "fmt"

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.stdout, format, args...)
}

func (a *App) print(args ...any) {
	fmt.Fprint(a.stdout, args...)
}

func (a *App) println(args ...any) {
	fmt.Fprintln(a.stdout, args...)
}
