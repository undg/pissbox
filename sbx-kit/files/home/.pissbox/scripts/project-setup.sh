#!/usr/bin/env bash

set -euo pipefail

VALIDATE_ONLY=false
CONFIG=""

while [[ $# -gt 0 ]]; do
	case $1 in
	--validate)
		if [[ -z ${2:-} || ${2:-} == -* ]]; then
			echo "--validate requires a TOML file" >&2
			exit 1
		fi
		VALIDATE_ONLY=true
		CONFIG=$2
		shift 2
		;;
	*)
		echo "Usage: ${0##*/} [--validate FILE]" >&2
		exit 1
		;;
	esac
done

validate_config() {
	local config=$1
	if [[ ! -f $config || ! -r $config ]]; then
		echo "Project configuration must be a readable TOML file: $config" >&2
		return 1
	fi
	tomljson "$config" | jq -e '
		if type != "object" then
			error("Project configuration must be a TOML table")
		elif (keys - ["setup", "workspaces"] | length) != 0 then
			error("Unknown project configuration key; supported keys: setup, workspaces")
		elif has("setup") and (.setup | type) != "array" then
			error("setup must be an array of command strings")
		elif (.setup // [] | all(.[]; type == "string")) | not then
			error("setup must contain only command strings")
		elif (.setup // [] | any(.[]; index("\u0000") != null)) then
			error("setup commands must not contain NUL bytes")
		elif has("workspaces") and (.workspaces | type) != "array" then
			error("workspaces must be an array of directory paths")
		elif (.workspaces // [] | all(.[]; type == "string")) | not then
			error("workspaces must contain only directory path strings")
		elif (.workspaces // [] | any(.[]; length == 0 or test("[\r\n]") or index("\u0000") != null)) then
			error("workspaces paths must not be empty or contain NUL bytes or line breaks")
		else
			. + {setup: (.setup // []), workspaces: (.workspaces // [])}
		end
	'
}

# The host reads validated data only; it never runs project setup commands.
if [[ $VALIDATE_ONLY == true ]]; then
	validate_config "$CONFIG"
	exit 0
fi

WORKSPACE=${PISSBOX_WORKSPACE_DIR:?Set PISSBOX_WORKSPACE_DIR to the primary workspace}
case $WORKSPACE in
/*) ;;
*) echo "Primary workspace path must be absolute: $WORKSPACE" >&2; exit 1 ;;
esac

cd -- "$WORKSPACE"
CONFIG="$PWD/.pissbox.toml"
if [[ ! -e $CONFIG && ! -L $CONFIG ]]; then
	exit 0
fi

# Parse and validate everything before any project command can run.
CONFIG_JSON=$(validate_config "$CONFIG")
COMMANDS=$(printf '%s\n' "$CONFIG_JSON" | jq -r '.setup | join("\n")')
if [[ -z $COMMANDS ]]; then
	exit 0
fi

printf 'Running project setup from: %s\n' "$CONFIG" >&2
exec bash --noprofile --norc -euo pipefail -c "$COMMANDS" "$CONFIG"
