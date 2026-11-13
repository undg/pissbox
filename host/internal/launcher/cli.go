// Package launcher implements the short-lived host launcher, independently of the bridge daemon.
package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type options struct {
	name, deleteDir, piDir, stateDir, completion                       string
	yes, list, bridgeStop, bridgeReload, ignoreConfig, help, continued bool
	recreate                                                           bool
	workspaces                                                         []string
	secrets                                                            []secret
}

// newCLI defines flags once for parsing, help, and shell completion. Ordinary
// invocations use ParseFlags, not Execute, so no word is reserved as a command.
func newCLI(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "pissbox [OPTIONS] [WORKSPACE ...]",
		DisableFlagsInUseLine: true,
		Short:                 "Start Pi in an SBX sandbox.",
		Example:               "  pissbox\n  pissbox --continue\n  pissbox --secret GH_TOKEN api.github.com '/usr/local/bin/get-token'\n  pissbox --completion zsh",
		Args:                  cobra.ArbitraryArgs,
		SilenceUsage:          true,
		SilenceErrors:         true,
		CompletionOptions:     cobra.CompletionOptions{DisableDefaultCmd: true},
		ValidArgsFunction:     completeDirectories,
		Run:                   func(*cobra.Command, []string) {},
	}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetHelpTemplate(helpTemplate)
	f := cmd.Flags()
	f.SortFlags = false
	f.BoolVarP(&o.continued, "continue", "c", false, "Continue the previous Pi session without the menu")
	f.StringVarP(&o.deleteDir, "delete", "d", "", "Delete matching sandboxes for `DIR`; loses sandbox-local data")
	f.BoolVar(&o.recreate, "recreate", false, "Replace the matching sandbox with current config; loses sandbox-local data")
	f.BoolVarP(&o.yes, "yes", "y", false, "Skip the existing-sandbox menu or deletion confirmation")
	f.BoolVarP(&o.list, "ls", "l", false, "List sandboxes, current-directory matches first")
	f.StringVar(&o.name, "name", "", "Use an explicit sandbox `NAME`")
	f.Var(&secretFlag{secrets: &o.secrets}, "secret", "Register a proxy secret on creation only (repeatable): `ENV_NAME HOST COMMAND`")
	f.BoolVar(&o.ignoreConfig, "ignore-config-workspaces", false, "Skip configured mounts; keep CLI paths and setup")
	f.StringVar(&o.piDir, "pissbox-pi-dir", "", "Override the shared sandbox Pi configuration directory: `DIR`")
	f.StringVar(&o.stateDir, "pissbox-state-dir", "", "Override the shared Linux tool-state directory: `DIR`")
	f.BoolVar(&o.bridgeStop, "bridge-stop", false, "Stop the compatible host bridge daemon")
	f.BoolVar(&o.bridgeReload, "bridge-reload", false, "Restart the compatible bridge without releasing agents")
	f.StringVar(&o.completion, "completion", "", "Generate shell integration for `SHELL` (bash, zsh, fish, powershell)")
	f.BoolVarP(&o.help, "help", "h", false, "Show this help")
	for _, name := range []string{"delete", "pissbox-pi-dir", "pissbox-state-dir"} {
		if err := cmd.RegisterFlagCompletionFunc(name, completeDirectories); err != nil {
			panic(err) // These names are defined above, once per command.
		}
	}
	if err := cmd.RegisterFlagCompletionFunc("completion", completeShells); err != nil {
		panic(err)
	}
	if err := cmd.RegisterFlagCompletionFunc("secret", noCompletion); err != nil {
		panic(err)
	}
	if err := cmd.RegisterFlagCompletionFunc("name", noCompletion); err != nil {
		panic(err)
	}
	return cmd
}

func parse(args []string) (options, error) {
	var o options
	cmd := newCLI(&o)
	normalized, err := normalizeArgs(cmd, args, false)
	if err != nil {
		return o, err
	}
	if err := cmd.ParseFlags(normalized); err != nil {
		return o, err
	}
	o.workspaces = cmd.Flags().Args()
	if o.recreate && (o.deleteDir != "" || o.list || o.bridgeStop || o.bridgeReload) {
		return o, fmt.Errorf("--recreate cannot be combined with list, delete, bridge-stop, or bridge-reload")
	}
	return o, nil
}

// normalizeArgs preserves the three-argument --secret interface and the early
// help exit. Other flags are parsed by Cobra. Flag values are skipped so words
// inside them cannot accidentally become options.
func normalizeArgs(cmd *cobra.Command, args []string, completing bool) ([]string, error) {
	var normalized []string
	var secrets []secret
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--":
			return append(normalized, args[i:]...), nil
		case "-h", "--help":
			return append(normalized, arg), nil
		case "--secret":
			if i+3 >= len(args) {
				return nil, fmt.Errorf("--secret requires ENV_NAME HOST COMMAND")
			}
			entry := secret{Env: args[i+1], Host: args[i+2], Command: args[i+3]}
			if err := entry.Validate(secrets); err != nil {
				return nil, err
			}
			secrets = append(secrets, entry)
			if !completing {
				data, err := json.Marshal(entry)
				if err != nil {
					return nil, err
				}
				normalized = append(normalized, "--secret="+string(data))
			}
			// Completion needs no secret values. Keep commands out of Cobra's
			// diagnostics, including errors about later flags.
			i += 3
			continue
		case "--bridge-stop", "--bridge-reload":
			if !completing && i != len(args)-1 {
				return nil, fmt.Errorf("%s must be used by itself", arg)
			}
		}
		name, value, equals := strings.Cut(arg, "=")
		// The legacy interface only accepts the three-word secret form.
		if name == "--secret" {
			return nil, fmt.Errorf("--secret requires ENV_NAME HOST COMMAND")
		}
		if !strings.HasPrefix(name, "-") {
			normalized = append(normalized, arg)
			continue
		}
		flag := cmd.Flags().Lookup(strings.TrimPrefix(name, "--"))
		if len(name) == 2 && name[0] == '-' {
			flag = cmd.Flags().ShorthandLookup(name[1:])
		}
		if flag != nil && flag.NoOptDefVal == "" {
			if !equals {
				i++
				if i < len(args) {
					value = args[i]
				} else if completing {
					return append(normalized, arg), nil
				}
			}
			if value == "" || (!equals && strings.HasPrefix(value, "-")) {
				metavar := "a directory"
				if flag.Name == "name" {
					metavar = "a sandbox name"
				} else if flag.Name == "completion" {
					metavar = "a shell name"
				}
				return nil, fmt.Errorf("%s requires %s", name, metavar)
			}
			normalized = append(normalized, "--"+flag.Name+"="+value)
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized, nil
}

// secretFlag consumes the internal JSON produced by normalizeArgs, never shell
// syntax. Commands remain opaque and are validated without being executed.
type secretFlag struct{ secrets *[]secret }

func (s *secretFlag) String() string { return "" }
func (s *secretFlag) Type() string   { return "secretArray" }
func (s *secretFlag) Set(value string) error {
	var entry secret
	if err := json.Unmarshal([]byte(value), &entry); err != nil {
		return fmt.Errorf("--secret requires ENV_NAME HOST COMMAND")
	}
	// normalizeArgs validates entries before pflag can include their values
	// in an error message. Completion drops secret triples altogether.
	*s.secrets = append(*s.secrets, entry)
	return nil
}

func (o options) piArgs() []string {
	if o.continued {
		return []string{"--", "--continue"}
	}
	return nil
}
