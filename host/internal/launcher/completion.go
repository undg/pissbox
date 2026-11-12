package launcher

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func completeDirectories(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func noCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeShells(_ *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if strings.HasPrefix(shell, prefix) {
			matches = append(matches, shell)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func writeCompletion(shell string, out io.Writer) error {
	cmd := newCLI(&options{})
	var script bytes.Buffer
	var err error
	switch shell {
	case "bash":
		err = cmd.GenBashCompletionV2(&script, true)
	case "zsh":
		err = cmd.GenZshCompletion(&script)
	case "fish":
		err = cmd.GenFishCompletion(&script, true)
	case "powershell":
		err = cmd.GenPowerShellCompletionWithDesc(&script)
	default:
		return fmt.Errorf("unsupported completion shell %q; choose bash, zsh, fish, or powershell", shell)
	}
	if err != nil {
		return err
	}
	// Use a flag for Cobra's private completion protocol too. Bare __complete
	// and __completeNoDesc must remain usable as workspace directory names.
	text := strings.ReplaceAll(script.String(), cobra.ShellCompNoDescRequestCmd, "--complete")
	text = strings.ReplaceAll(text, cobra.ShellCompRequestCmd, "--complete")
	_, err = io.WriteString(out, text)
	return err
}

// requestCompletion is entered only through the leading internal --complete
// flag. It never loads config, probes commands, or enters the launch lifecycle.
func requestCompletion(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("--complete requires a word to complete (which may be empty)")
	}
	cmd := newCLI(&options{})
	normalized, err := normalizeArgs(cmd, args[:len(args)-1], true)
	if err != nil {
		// An incomplete secret triple (or invalid earlier input) must not leak
		// commands into diagnostics or trigger unrelated filesystem completion.
		_, err = fmt.Fprintf(out, ":%d\n", cobra.ShellCompDirectiveNoFileComp)
		return err
	}
	cmd.SetOut(out)
	cmd.SetArgs(append([]string{cobra.ShellCompRequestCmd}, append(normalized, args[len(args)-1])...))
	return cmd.Execute()
}
