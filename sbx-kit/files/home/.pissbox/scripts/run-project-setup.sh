#!/usr/bin/env bash

set -euo pipefail

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
if [[ ! -f $CONFIG || ! -r $CONFIG ]]; then
	echo "Project configuration must be a readable TOML file: $CONFIG" >&2
	exit 1
fi

# Parse and validate everything before any project command can run.
COMMANDS=$(tomljson "$CONFIG" | jq -er '
	if (keys - ["setup"] | length) != 0 then
		error("Unknown project configuration key; supported keys: setup")
	elif has("setup") | not then
		""
	elif (.setup | type) != "array" then
		error("setup must be an array of command strings")
	elif (.setup | all(.[]; type == "string")) | not then
		error("setup must contain only command strings")
	elif (.setup | any(.[]; contains("\u0000"))) then
		error("setup commands must not contain NUL bytes")
	else
		.setup | join("\n")
	end
')
if [[ -z $COMMANDS ]]; then
	exit 0
fi

printf 'Running project setup from: %s\n' "$CONFIG" >&2
exec bash --noprofile --norc -euo pipefail -c "$COMMANDS" "$CONFIG"
