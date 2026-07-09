// Command charon detects the Codex, Claude Code and OpenCode CLIs and switches
// their endpoint + credentials between saved profiles.
package main

import (
	"fmt"
	"os"

	"charon/internal/profile"
	"charon/internal/tools"
	"charon/internal/tui"
)

// version is set at build time via -ldflags (see .goreleaser.yaml).
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "charon: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "update":
			return cmdUpdate()
		case "uninstall":
			return cmdUninstall()
		case "version", "-v", "--version":
			fmt.Println("charon " + version)
			return nil
		case "help", "-h", "--help":
			printUsage()
			return nil
		}
	}

	store, err := profile.Open()
	if err != nil {
		return err
	}
	// Capture the pristine config of every detected tool on first sight.
	for _, t := range tools.All() {
		if err := store.EnsureDefault(t); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not snapshot %s: %v\n", t.Name, err)
		}
	}

	if len(args) == 0 {
		return tui.Run(store, version)
	}

	switch args[0] {
	case "status", "st":
		return cmdStatus(store, args[1:])
	case "ls":
		return cmdList(store, args[1:])
	case "switch", "use":
		return cmdSwitch(store, args[1:])
	case "restore":
		return cmdSwitch(store, append([]string{argAt(args, 1)}, profile.DefaultName))
	case "undo":
		return cmdUndo(store, args[1:])
	case "prune":
		return cmdPrune(store, args[1:])
	case "save":
		return cmdSave(store, args[1:])
	case "models":
		return cmdModels(args[1:])
	case "add":
		return cmdAdd(store, args[1:])
	case "edit":
		return cmdEdit(store, args[1:])
	case "rename", "mv":
		return cmdRename(store, args[1:])
	case "cp":
		return cmdDuplicate(store, args[1:])
	case "rm":
		return cmdRemove(store, args[1:])
	case "completion":
		return cmdCompletion(args[1:])
	case "__profiles": // hidden: feeds shell completion
		return cmdProfiles(store, args[1:])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func printUsage() {
	fmt.Print(`charon — detect and switch AI tool endpoints + credentials

Usage:
  charon                     interactive menu
  charon status              show each tool's active profile, endpoint and auth (--json)
  charon ls <tool>           list saved profiles for a tool (--json)
  charon save <tool> [name]  snapshot current live config as a profile
                             (omit name to auto-name after the logged-in account)
  charon models <tool>       list models from an API (--key, --endpoint)
  charon add <tool>          add+activate a profile (--name --key [--endpoint --model])
  charon edit <tool> <p>     change a profile's endpoint/key/model/name
  charon rename <tool> <o> <n>  rename a saved profile
  charon cp <tool> <src> <dst>  duplicate a saved profile
  charon switch <tool> <p>   apply a saved profile (backs up current first)
  charon restore <tool>      revert to the auto-captured default
  charon undo <tool>         revert to the most recent pre-switch backup
  charon prune <tool>        delete old backups, keeping the newest (--keep N)
  charon rm <tool> <p>       delete a saved profile
  charon completion <shell>  print a bash/zsh/fish completion script
  charon update              upgrade charon to the latest version
  charon uninstall           remove the installed charon binary

Tools: codex, claude, opencode
`)
}
