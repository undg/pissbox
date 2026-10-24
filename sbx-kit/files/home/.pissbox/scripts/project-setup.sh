#!/usr/bin/env bash

set -euo pipefail

VALIDATE_ONLY=false
FINALIZE_ISOLATION=false
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
	--finalize-isolation)
		FINALIZE_ISOLATION=true
		shift
		;;
	*)
		echo "Usage: ${0##*/} [--validate FILE | --finalize-isolation]" >&2
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
		elif (keys - ["setup", "workspaces", "isolated_directories"] | length) != 0 then
			error("Unknown project configuration key; supported keys: setup, workspaces, isolated_directories")
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
		elif has("isolated_directories") and (.isolated_directories | type) != "array" then
			error("isolated_directories must be an array of relative directory paths")
		elif (.isolated_directories // [] | all(.[]; type == "string")) | not then
			error("isolated_directories must contain only relative directory path strings")
		elif (.isolated_directories // [] | any(.[];
			length == 0 or startswith("/") or
			(explode | any(.[]; . < 32 or (. >= 127 and . <= 159) or . == 58)) or
			(split("/") | any(.[]; . == "" or . == "." or . == "..")))) then
			error("isolated_directories paths must be normalized relative directories without colons or control characters")
		elif (.isolated_directories // []) as $paths |
			any(range(0; $paths | length); . as $i |
				any(range($i + 1; $paths | length); . as $j |
					$paths[$i] == $paths[$j] or
					($paths[$i] | startswith($paths[$j] + "/")) or
					($paths[$j] | startswith($paths[$i] + "/")))) then
			error("isolated_directories paths must not duplicate or overlap")
		else
			. + {setup: (.setup // []), workspaces: (.workspaces // []),
				isolated_directories: (.isolated_directories // [])}
		end
	'
}

# The host reads validated data only; it never runs project setup commands.
if [[ $VALIDATE_ONLY == true ]]; then
	validate_config "$CONFIG"
	exit 0
fi

if [[ -n ${PISSBOX_ISOLATION_DIR:-} ]]; then
	if [[ $FINALIZE_ISOLATION == false ]]; then
		if [[ ! ${SANDBOX_NAME:-} =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]]; then
			echo "Isolation requires a valid SANDBOX_NAME during kit installation." >&2
			exit 1
		fi
		printf '%s\n' "$SANDBOX_NAME" > "$PISSBOX_ISOLATION_DIR/name.next"
		mv "$PISSBOX_ISOLATION_DIR/name.next" "$PISSBOX_ISOLATION_DIR/name"
		exit 0
	fi
	# Do not replay arbitrary setup after a failed or interrupted finalization.
	(set -C; : > "$HOME/.pissbox/isolation-started")
elif [[ $FINALIZE_ISOLATION == true ]]; then
	echo "Cannot finalize isolation without its persisted creation environment." >&2
	exit 1
fi

WORKSPACE=${PISSBOX_WORKSPACE_DIR:?Set PISSBOX_WORKSPACE_DIR to the primary workspace}
case $WORKSPACE in
/*) ;;
*) echo "Primary workspace path must be absolute: $WORKSPACE" >&2; exit 1 ;;
esac

cd -- "$WORKSPACE"
CONFIG="$PWD/.pissbox.toml"
COMMANDS=""
if [[ -e $CONFIG || -L $CONFIG ]]; then
	# Parse and validate everything before any project command can run.
	CONFIG_JSON=$(validate_config "$CONFIG")
	ISOLATED_COUNT=$(printf '%s\n' "$CONFIG_JSON" | jq '.isolated_directories | length')
	if [[ $ISOLATED_COUNT -gt 0 && -z ${PISSBOX_ISOLATION_DIR:-} ]]; then
		echo "Project requires isolated directories; launch via the pissbox isolation adapter before running setup." >&2
		exit 1
	fi
	COMMANDS=$(printf '%s\n' "$CONFIG_JSON" | jq -r '.setup | join("\n")')
fi
if [[ -n $COMMANDS ]]; then
	printf 'Running project setup from: %s\n' "$CONFIG" >&2
	if [[ $FINALIZE_ISOLATION == false ]]; then
		exec bash --noprofile --norc -euo pipefail -c "$COMMANDS" "$CONFIG"
	fi
	bash --noprofile --norc -euo pipefail -c "$COMMANDS" "$CONFIG"
fi
if [[ $FINALIZE_ISOLATION == true ]]; then
	printf '%s\n' "$PISSBOX_ISOLATION_DIR" > "$HOME/.pissbox/isolation-ready"
fi
