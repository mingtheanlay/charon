// Command charon detects the Codex, Claude Code and OpenCode CLIs and switches
// their endpoint + credentials between saved profiles.
package main

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"charon/internal/models"
	"charon/internal/profile"
	"charon/internal/secret"
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
	store, err := profile.Open()
	if err != nil {
		return err
	}
	// Capture the pristine config of every detected tool on first sight.
	for _, t := range tools.All() {
		if err := store.EnsureOriginal(t); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not snapshot %s: %v\n", t.Name, err)
		}
	}

	if len(args) == 0 {
		return tui.Run(store)
	}

	switch args[0] {
	case "status", "st":
		return cmdStatus(store)
	case "ls":
		return cmdList(store, args[1:])
	case "switch", "use":
		return cmdSwitch(store, args[1:])
	case "restore":
		return cmdSwitch(store, append([]string{argAt(args, 1)}, profile.OriginalName))
	case "save":
		return cmdSave(store, args[1:])
	case "models":
		return cmdModels(args[1:])
	case "add":
		return cmdAdd(store, args[1:])
	case "rm":
		return cmdRemove(store, args[1:])
	case "version", "-v", "--version":
		fmt.Println("charon " + version)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
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

func cmdStatus(store *profile.Store) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tACTIVE\tAUTH\tENDPOINT\tSECRET")
	for _, t := range tools.All() {
		if t.Detected == nil || !t.Detected() {
			fmt.Fprintf(w, "%s\t—\t(not detected)\t\t\n", t.Title)
			continue
		}
		info, _ := t.Describe()
		active := store.Active(t.Name)
		if active == "" {
			active = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			t.Title, active, info.AuthMode, info.Endpoint, secret.Mask(info.Secret))
	}
	return w.Flush()
}

func cmdList(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon ls <tool>")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	active := store.Active(t.Name)
	for _, name := range store.List(t.Name) {
		marker := "  "
		if name == active {
			marker = "* "
		}
		m, _ := store.LoadManifest(t.Name, name)
		fmt.Printf("%s%s\t%s\n", marker, name, m.Label)
	}
	return nil
}

func cmdSwitch(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon switch <tool> <profile>")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	backup, err := store.Apply(t, args[1])
	if err != nil {
		return err
	}
	fmt.Printf("Switched %s → %s\n(backup: %s)\n", t.Title, args[1], backup)
	return nil
}

func cmdSave(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon save <tool> <name> [--label ..] [--note ..]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	name := args[1]
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	label := fs.String("label", "", "human-friendly label")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if err := store.Save(t, name, *label, *note); err != nil {
		return err
	}
	fmt.Printf("Saved current %s config as profile %q\n", t.Title, name)
	return nil
}

func cmdModels(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon models <tool> --key <key> [--endpoint <url>]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	list, err := models.Fetch(models.Provider(t.Provider), t.ResolveEndpoint(*endpoint), *key)
	if err != nil {
		return err
	}
	for _, m := range list {
		fmt.Println(m)
	}
	return nil
}

func cmdAdd(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon add <tool> --name <p> --key <k> [--endpoint <url>] [--model <m>]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	endpoint := fs.String("endpoint", "", "API base URL")
	key := fs.String("key", "", "API key")
	model := fs.String("model", "", "model id")
	name := fs.String("name", "", "profile name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if t.ApplyAuth == nil {
		return fmt.Errorf("%s does not support add", t.Title)
	}
	if *name == "" || *key == "" {
		return fmt.Errorf("--name and --key are required")
	}
	ep := t.ResolveEndpoint(*endpoint)
	if err := store.AddProfile(t, *name, profile.Spec{Endpoint: ep, Key: *key, Model: *model}); err != nil {
		return err
	}
	fmt.Printf("Added and activated %s profile %q (%s · %s)\n", t.Title, *name, ep, *model)
	return nil
}

func cmdRemove(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon rm <tool> <profile>")
	}
	if err := store.Remove(args[0], args[1]); err != nil {
		return err
	}
	fmt.Printf("Removed profile %q for %s\n", args[1], args[0])
	return nil
}

func printUsage() {
	fmt.Print(`charon — detect and switch AI tool endpoints + credentials

Usage:
  charon                     interactive menu
  charon status              show each tool's active profile, endpoint and auth
  charon ls <tool>           list saved profiles for a tool
  charon save <tool> <name>  snapshot current live config as a profile
  charon models <tool>       list models from an API (--key, --endpoint)
  charon add <tool>          add+activate a profile (--name --key [--endpoint --model])
  charon switch <tool> <p>   apply a saved profile (backs up current first)
  charon restore <tool>      revert to the auto-captured original
  charon rm <tool> <p>       delete a saved profile

Tools: codex, claude, opencode
`)
}
