---
name: bash-cli-style
description: Create, refactor, or review Bash command-line scripts in the user's preferred style. Use for launchers, installers, wrappers, argument parsing, help text, and shell-script cleanup.
---

# Bash CLI style

Write straightforward shell that is easy to read and change. Preserve existing
behavior unless the user asks to change it. Do not add options, environment
variables, functions, or dependencies merely to fit a template.

## Architecture: configure first, execute afterward

The `case` loop is a configuration phase, not an action dispatcher. Its branches
set variables and collect arguments. Only after parsing finishes does the script
validate the final configuration and execute the requested operations.

```text
defaults → parse all arguments into state → validate → execute
```

- Flags need not be Boolean. Store the appropriate value: a Boolean switch, a path, a mode, a number, or an array.
- An option may set several variables. In the reference installer, `--stow-only` enables stow and disables other operations; it does not run stow inside the parser.
- Accept multiple independent options in arbitrary order. Do not launch commands, install packages, or perform other operational work as each option is encountered.
- Keep the execution section after the loop. It reads the final variables and decides which functions to call or which arguments to pass to the wrapped command.
- Help and parse errors may exit immediately; they are not operational work.
- For repeated or conflicting options, define predictable semantics, usually the last assignment wins. Arbitrary ordering of independent options does not mean conflicting options must produce identical results in every order.
- Keep positional arguments separate from option state. For a launcher, collect workspace paths in an array during parsing, then apply defaults and pass that array to the final command after parsing. Positionals often appear at the end, but encountering one must not prevent later options from being parsed.

This separation is the reason for the architecture: adding another option should
usually mean adding a default, a `case` branch that sets state, and any necessary
handling in the final execution section—not coupling behavior to argument order.

## Structure

- Use `#!/usr/bin/env bash`, a blank line, then `set -euo pipefail` for new standalone Bash scripts. Handle optional values and expected failures explicitly.
- Use tabs for indentation and ordinary `function_name() { ...; }` function declarations.
- Use uppercase script-level variables, such as `SCRIPT_DIR`, `CONFIG_DIR`, and `WORKSPACES`. Use lowercase `local` variables inside functions.
- Prefer this order: configuration defaults; help and any parser helpers; argument parsing; operational helper functions; validation and derived configuration; execution.
- Define functions before their first call: Bash does not hoist function definitions. Keep parser helpers above the argument loop and operational helpers together below it.
- Keep literal configuration defaults near the top. Derive values that require parsed options, filesystem checks, or external commands after parsing, so `--help` does not depend on the runtime environment.
- Give distinct operations small, named functions. A short wrapper does not need a function around its single final command.
- Keep execution flow visible. Avoid generic parsing frameworks, unnecessary abstractions, and clever one-liners.
- Keep comments short. Explain a surprising decision or constraint, not what every command does. No historical essays or speculative infrastructure claims.

## Help

Use a `show_help()` function with `cat <<EOF`. Do not assemble help with a series
of `printf` or `echo` calls. Include usage, supported options, and relevant defaults.
Only document options the script actually supports.

Illustrative shape; replace the example option with the script's real options:

```bash
show_help() {
	cat <<EOF
Usage: ${0##*/} [OPTIONS] [PATH ...]

One sentence explaining the command.

Options:
  --output-dir DIR  Override OUTPUT_DIR
  -h, --help        Show this help
  --                Treat remaining arguments as paths

OUTPUT_DIR defaults to the current directory.
EOF
}
```

Allow deliberate expansion, such as `${0##*/}`, in the help heredoc. Escape any
text that must remain literal, or use `<<'EOF'` when no expansion is needed.
Help must exit successfully without launching the wrapped command, validating
runtime paths, or requiring external dependencies.

## Arguments and defaults

- Parse arguments with `while [[ $# -gt 0 ]]; do` and `case $1 in ... esac`.
- Handle each supported option in its own case branch, updating configuration rather than performing the requested operation. Use `shift` for a flag and `shift 2` for an option with a separate value.
- Recognize `-h | --help` wherever options are accepted, not just as the first argument.
- For value-taking options, reject missing or empty values clearly. Do not accidentally consume the next option as a value. For dash-prefixed paths, use an explicit form such as `./-name` or `--option=-name` when supported.
- Accept positionals into an array, preserving order and spaces. Allow options between positionals unless the command deliberately requires a different interface.
- Support `--` when accepting positional paths: shift it, append the remaining arguments unchanged, then stop parsing. Flags after it are positional data.
- Reject unknown options on stderr with a nonzero exit status and a hint to use `--help`.
- Support `--option=value` when useful; do not add aliases or alternate spellings without a reason.
- When a setting has a CLI flag and an environment variable, use **CLI flag > environment variable > built-in default**. Initialize the default before parsing so flags override it.
- Validate the selected value after parsing. An invalid environment default must not prevent a valid CLI override or `--help` from working.
- Explain precedence and how relative paths resolve. Do not silently invent additional configuration sources.

## Paths and command execution

- Derive checkout-relative resources from `BASH_SOURCE[0]`, never a machine-specific absolute path or assumed repository directory name.
- Use `$HOME` only for the home directory in the environment executing the script. It does not discover a host home from inside a container.
- Preserve the caller's working directory when it determines the workspace. Do not copy an installer's `cd "$SCRIPT_DIR"` into a launcher blindly.
- Resolve relative user paths from the caller's working directory. Canonicalize existing directory paths with `cd ... && pwd -P` when the downstream command needs an absolute path.
- Quote path expansions. Forward positional arguments as `"$@"` or `"${ARGS[@]}"`, not joined strings. Do not use `eval` to construct commands.
- Use `${OPTIONAL_VALUE:-}` or an explicit default when reading optional variables under `set -u`.
- Use `exec` for the final command in a thin wrapper, preserving its exit status and signal handling.
- Send errors and diagnostics to stderr; keep result output on stdout. Do not add colors, banners, or progress logging unless useful or requested.
- Keep the script focused. Prefer the underlying tool's supported behavior over custom discovery, state management, or fallback machinery.
- For scripts targeting macOS's system Bash, avoid features that require newer Bash unless the version requirement is explicit.

## Verification

Run `bash -n` and, when available, ShellCheck. In a repository, run
`git diff --check`. Test the actual script rather than a reimplementation of its
parser. Use a stub for the wrapped command when launching it would have effects.

Cover relevant cases:

- Both help flags, including help after a positional argument.
- Default values, environment overrides, and CLI-over-environment precedence.
- Missing and empty option values, unknown options, and nonexistent paths.
- Multiple independent options in different orders, producing the same final configuration and execution.
- Repeated or conflicting options following the documented precedence rules.
- Multiple positional paths, paths with spaces, options before and after positionals, and `--` handling.
- Caller working-directory preservation and correct argument forwarding.
- Failure exit statuses and stderr messages.

Do not claim a real external-command or environment integration test when only
syntax checks or stubbed argument tests were run.

## Style reference

The user's reference is [undg/.dot/install](https://github.com/undg/.dot/blob/master/install).
Use its architecture, not just its syntax: options configure variables, and a
final execution section acts on that state. Also follow its straightforward
functions, uppercase script-level variables, EOF help, and `case` parsing.
Do not copy its package lists, destructive operations, global directory changes,
or task-specific behavior.
