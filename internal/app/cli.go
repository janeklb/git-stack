package app

import (
	"fmt"
	"os"
)

func (a *App) Run(args []string, invocation string) int {
	if len(args) == 0 {
		a.printHelp(invocation)
		return 0
	}

	cmd := args[0]
	var err error
	switch cmd {
	case "help", "-h", "--help":
		a.printHelp(invocation)
		return 0
	case "init":
		err = a.cmdInit(args[1:])
	case "new":
		err = a.cmdNew(args[1:])
	case "status":
		err = a.cmdStatus(args[1:])
	case "restack":
		err = a.cmdRestack(args[1:])
	case "submit":
		err = a.cmdSubmit(args[1:])
	case "reparent":
		err = a.cmdReparent(args[1:])
	case "repair":
		err = a.cmdRepair(args[1:])
	default:
		err = fmt.Errorf("unknown command: %s", cmd)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func (a *App) printHelp(invocation string) {
	bin := invocation
	if bin == "" {
		bin = "stack"
	}
	fmt.Printf(`%s - stacked PR tool

Usage:
  %s init [--trunk <branch>] [--mode rebase|merge]
  %s new <name> [--parent <branch>] [--template <template>] [--prefix-index]
  %s status
  %s restack [--mode rebase|merge] [--continue] [--abort]
  %s submit [--all] [branch]
  %s reparent <branch> --parent <new-parent>
  %s repair

Equivalent git extension form:
  git stack <command>
`, bin, bin, bin, bin, bin, bin, bin, bin)
}
