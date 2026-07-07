// Command charon detects the Codex, Claude Code and OpenCode CLIs and switches
// their endpoint + credentials between saved profiles.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
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
		if err := store.EnsureDefault(t); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not snapshot %s: %v\n", t.Name, err)
		}
	}

	if len(args) == 0 {
		return tui.Run(store)
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
	case "rm":
		return cmdRemove(store, args[1:])
	case "completion":
		return cmdCompletion(args[1:])
	case "__profiles": // hidden: feeds shell completion
		return cmdProfiles(store, args[1:])
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

type statusRow struct {
	Tool     string `json:"tool"`
	Title    string `json:"title"`
	Detected bool   `json:"detected"`
	Active   string `json:"active,omitempty"`
	AuthMode string `json:"authMode,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Account  string `json:"account,omitempty"`
	Secret   string `json:"secret,omitempty"` // masked; never the raw value
	Modified bool   `json:"modified"`
}

func cmdStatus(store *profile.Store, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var rows []statusRow
	for _, t := range tools.All() {
		r := statusRow{Tool: t.Name, Title: t.Title}
		if t.Detected != nil && t.Detected() {
			info, _ := t.Describe()
			r.Detected = true
			r.Active = store.Active(t.Name)
			r.AuthMode = info.AuthMode
			r.Endpoint = info.Endpoint
			r.Model = info.Model
			r.Account = info.Account
			r.Secret = secret.Mask(info.Secret)
			r.Modified, _ = store.Drift(t)
		}
		rows = append(rows, r)
	}

	if *asJSON {
		return printJSON(rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tACTIVE\tAUTH\tENDPOINT\tSECRET")
	for _, r := range rows {
		if !r.Detected {
			fmt.Fprintf(w, "%s\t—\t(not detected)\t\t\n", r.Title)
			continue
		}
		active := r.Active
		if active == "" {
			active = "—"
		}
		if r.Modified {
			active += " (modified)" // live config changed since the last switch
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Title, active, r.AuthMode, r.Endpoint, r.Secret)
	}
	return w.Flush()
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type profileRow struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Account  string `json:"account,omitempty"`
}

func cmdList(store *profile.Store, args []string) error {
	toolName, rest := splitTool(args)
	if toolName == "" {
		return fmt.Errorf("usage: charon ls <tool> [--json]")
	}
	t := tools.Find(toolName)
	if t == nil {
		return fmt.Errorf("unknown tool %q", toolName)
	}
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "machine-readable JSON output")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	active := store.Active(t.Name)
	var rows []profileRow
	for _, name := range store.List(t.Name) {
		m, _ := store.LoadManifest(t.Name, name)
		r := profileRow{Name: name, Label: m.Label, Account: m.Account, Active: name == active}
		if sp, ok := store.GetSpec(t.Name, name); ok {
			r.Endpoint = t.ResolveEndpoint(sp.Endpoint)
			r.Model = sp.Model
		}
		rows = append(rows, r)
	}

	if *asJSON {
		return printJSON(rows)
	}
	for _, r := range rows {
		marker := "  "
		if r.Active {
			marker = "* "
		}
		fmt.Printf("%s%s\t%s\n", marker, r.Name, r.Label)
	}
	return nil
}

// splitTool returns the first non-flag arg (the tool) and the remaining args.
func splitTool(args []string) (tool string, rest []string) {
	for _, a := range args {
		if tool == "" && !strings.HasPrefix(a, "-") {
			tool = a
			continue
		}
		rest = append(rest, a)
	}
	return tool, rest
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
	if len(args) < 1 {
		return fmt.Errorf("usage: charon save <tool> [name] [--label ..] [--note ..]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	// A bare name (not a flag) is optional; without it, name after the logged-in account.
	rest := args[1:]
	name := ""
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		name, rest = rest[0], rest[1:]
	}
	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	label := fs.String("label", "", "human-friendly label")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if name == "" {
		saved, err := store.SaveCurrentAccount(t)
		if err != nil {
			return err
		}
		fmt.Printf("Backed up current %s account as profile %q (now active)\n", t.Title, saved)
		return nil
	}
	if err := store.Save(t, name, *label, *note); err != nil {
		return err
	}
	fmt.Printf("Saved current %s config as profile %q\n", t.Title, name)
	return nil
}

func cmdUndo(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon undo <tool>")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	restored, err := store.Undo(t)
	if err != nil {
		return err
	}
	fmt.Printf("Reverted %s to its most recent backup\n(restored: %s)\n", t.Title, restored)
	return nil
}

func cmdPrune(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon prune <tool> [--keep N]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	keep := fs.Int("keep", -1, "backups to retain (default 10)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	removed, err := store.PruneBackups(t.Name, *keep)
	if err != nil {
		return err
	}
	fmt.Printf("Pruned %d old %s backup(s)\n", removed, t.Title)
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

func cmdEdit(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon edit <tool> <profile> [--endpoint --key --model --name]")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	name := args[1]
	sp, ok := store.GetSpec(t.Name, name)
	if !ok {
		return fmt.Errorf("profile %q has no editable endpoint/key (captured or default profile)", name)
	}
	// Flags default to the current values, so an unset flag leaves that field unchanged.
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	endpoint := fs.String("endpoint", sp.Endpoint, "API base URL")
	key := fs.String("key", sp.Key, "API key")
	model := fs.String("model", sp.Model, "model id")
	newName := fs.String("name", "", "rename the profile")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	target := name
	if *newName != "" {
		target = *newName
	}
	if err := store.AddProfile(t, target, profile.Spec{Endpoint: *endpoint, Key: *key, Model: *model}); err != nil {
		return err
	}
	if target != name {
		_ = store.Remove(t.Name, name)
	}
	fmt.Printf("Updated %s profile %q (%s · %s)\n", t.Title, target, t.ResolveEndpoint(*endpoint), *model)
	return nil
}

func cmdRename(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: charon rename <tool> <old> <new>")
	}
	t := tools.Find(args[0])
	if t == nil {
		return fmt.Errorf("unknown tool %q", args[0])
	}
	if err := store.Rename(t.Name, args[1], args[2]); err != nil {
		return err
	}
	fmt.Printf("Renamed %s profile %q → %q\n", t.Title, args[1], args[2])
	return nil
}

func cmdCompletion(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: charon completion [bash|zsh|fish]")
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q (want bash, zsh, or fish)", args[0])
	}
	return nil
}

// cmdProfiles prints one profile name per line for a tool; used by shell completion.
func cmdProfiles(store *profile.Store, args []string) error {
	if len(args) < 1 {
		return nil
	}
	t := tools.Find(args[0])
	if t == nil {
		return nil
	}
	for _, name := range store.List(t.Name) {
		fmt.Println(name)
	}
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
  charon status              show each tool's active profile, endpoint and auth (--json)
  charon ls <tool>           list saved profiles for a tool (--json)
  charon save <tool> [name]  snapshot current live config as a profile
                             (omit name to auto-name after the logged-in account)
  charon models <tool>       list models from an API (--key, --endpoint)
  charon add <tool>          add+activate a profile (--name --key [--endpoint --model])
  charon edit <tool> <p>     change a profile's endpoint/key/model/name
  charon rename <tool> <o> <n>  rename a saved profile
  charon switch <tool> <p>   apply a saved profile (backs up current first)
  charon restore <tool>      revert to the auto-captured default
  charon undo <tool>         revert to the most recent pre-switch backup
  charon prune <tool>        delete old backups, keeping the newest (--keep N)
  charon rm <tool> <p>       delete a saved profile
  charon completion <shell>  print a bash/zsh/fish completion script

Tools: codex, claude, opencode
`)
}
