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
)

// requireTool resolves a tool-name argument, erroring with the supported names.
func requireTool(name string) (*tools.Tool, error) {
	if t := tools.Find(name); t != nil {
		return t, nil
	}
	var names []string
	for _, t := range tools.All() {
		names = append(names, t.Name)
	}
	if name == "" {
		return nil, fmt.Errorf("missing tool name (want %s)", strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("unknown tool %q (want %s)", name, strings.Join(names, ", "))
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

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type statusRow struct {
	Tool     string `json:"tool"`
	Title    string `json:"title"`
	Detected bool   `json:"detected"`
	Active   string `json:"active,omitempty"`
	AuthMode string `json:"authMode,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
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
			r.Effort = info.Effort
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
	fmt.Fprintln(w, "TOOL\tACTIVE\tAUTH\tENDPOINT\tMODEL\tEFFORT\tSECRET")
	for _, r := range rows {
		if !r.Detected {
			fmt.Fprintf(w, "%s\t—\t(not detected)\t\t\t\t\n", r.Title)
			continue
		}
		active := r.Active
		if active == "" {
			active = "—"
		}
		if r.Modified {
			active += " (modified)" // live config changed since the last switch
		}
		model, effort := r.Model, r.Effort
		if model == "" {
			model = "—"
		}
		if effort == "" {
			effort = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Title, active, r.AuthMode, r.Endpoint, model, effort, r.Secret)
	}
	return w.Flush()
}

type profileRow struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
	Account  string `json:"account,omitempty"`
}

func cmdList(store *profile.Store, args []string) error {
	toolName, rest := splitTool(args)
	if toolName == "" {
		return fmt.Errorf("usage: charon ls <tool> [--json]")
	}
	t, err := requireTool(toolName)
	if err != nil {
		return err
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
		sp, hasSpec := store.GetSpec(t.Name, name)
		if hasSpec {
			r.Endpoint = t.ResolveEndpoint(sp.Endpoint)
			r.Model = sp.Model
		}
		// ProfileModelEffort reads the profile's own captured config, which is more
		// accurate than the Add-time spec (e.g. reflects a later /model or /effort).
		if snapModel, snapEffort := store.ProfileModelEffort(t, name); snapModel != "" || snapEffort != "" {
			if snapModel != "" {
				r.Model = snapModel
			}
			r.Effort = snapEffort
		}
		rows = append(rows, r)
	}

	if *asJSON {
		return printJSON(rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tLABEL\tMODEL\tEFFORT")
	for _, r := range rows {
		marker := "  "
		if r.Active {
			marker = "* "
		}
		model, effort := r.Model, r.Effort
		if model == "" {
			model = "—"
		}
		if effort == "" {
			effort = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", marker, r.Name, r.Label, model, effort)
	}
	return w.Flush()
}

func cmdSwitch(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon switch <tool> <profile>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	t, err := requireTool(args[0])
	if err != nil {
		return err
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
	if err := store.EditProfile(t, name, target, profile.Spec{Endpoint: *endpoint, Key: *key, Model: *model}); err != nil {
		return err
	}
	fmt.Printf("Updated %s profile %q (%s · %s)\n", t.Title, target, t.ResolveEndpoint(*endpoint), *model)
	return nil
}

func cmdRename(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: charon rename <tool> <old> <new>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if err := store.Rename(t.Name, args[1], args[2]); err != nil {
		return err
	}
	fmt.Printf("Renamed %s profile %q → %q\n", t.Title, args[1], args[2])
	return nil
}

func cmdDuplicate(store *profile.Store, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: charon cp <tool> <src> <dst>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if err := store.Duplicate(t.Name, args[1], args[2]); err != nil {
		return err
	}
	fmt.Printf("Copied %s profile %q → %q\n", t.Title, args[1], args[2])
	return nil
}

func cmdRemove(store *profile.Store, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: charon rm <tool> <profile>")
	}
	t, err := requireTool(args[0])
	if err != nil {
		return err
	}
	if err := store.Remove(t.Name, args[1]); err != nil {
		return err
	}
	fmt.Printf("Removed profile %q for %s\n", args[1], t.Title)
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
