#!/usr/bin/env bash

# Link sandbox tool directories to shared Linux state under PISSBOX_STATE_DIR.
# ../shared-directories.conf maps state-relative|home-relative paths, one per row;
# blank lines and # comments are ignored. Paths are literal, without expansion.
# Create missing source directories; accept matching links, refuse existing data.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
STATE_DIR=${PISSBOX_STATE_DIR:?Set PISSBOX_STATE_DIR to the mounted host state directory}

if [[ $STATE_DIR != /* ]]; then
	echo "Shared state path must be absolute: $STATE_DIR" >&2
	exit 1
fi
if [[ ! -d $STATE_DIR ]]; then
	echo "Shared state mount is missing inside the sandbox: $STATE_DIR" >&2
	exit 1
fi

while IFS='|' read -r SOURCE TARGET || [[ -n $SOURCE ]]; do
	case $SOURCE in
	'' | \#*) continue ;;
	esac
	SOURCE="$STATE_DIR/$SOURCE"
	TARGET="$HOME/$TARGET"
	mkdir -p "$SOURCE"

	if [[ -L $TARGET ]]; then
		if [[ $(readlink "$TARGET") != "$SOURCE" ]]; then
			echo "Refusing to replace existing shared directory symlink: $TARGET" >&2
			exit 1
		fi
	elif [[ -e $TARGET ]]; then
		echo "Refusing to replace existing shared directory path: $TARGET" >&2
		exit 1
	else
		mkdir -p "$(dirname "$TARGET")"
		ln -s "$SOURCE" "$TARGET"
	fi
done < "$SCRIPT_DIR/../shared-directories.conf"
