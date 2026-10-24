# Sourced by pissbox only for opted-in project dependency isolation.

validate_isolated_paths() {
	local directory=$1 path part target remaining ancestor
	if [[ ! -d $directory ]]; then
		echo "Isolation requires an existing primary workspace: $directory" >&2
		return 1
	fi
	PISSBOX_WORKSPACE_DIR=$(cd -- "$directory" && pwd -P) || return $?
	for path in "$PISSBOX_WORKSPACE_DIR" "$PISSBOX_STATE_DIR"; do
		if ! printf '%s' "$path" | jq -Rse 'explode | all(.[]; . >= 32 and (. < 127 or . > 159) and . != 58)' >/dev/null; then
			echo "Isolation mount paths must not contain colons or control characters." >&2
			return 1
		fi
	done
	# Include physical ancestors when the selected state directory does not exist yet.
	ancestor=$PISSBOX_STATE_DIR
	while [[ ! -d $ancestor ]]; do
		ancestor=$(dirname -- "$ancestor") || return $?
	done
	path=$(cd -- "$ancestor" && pwd -P) || return $?
	if ! printf '%s' "$path" | jq -Rse 'explode | all(.[]; . >= 32 and (. < 127 or . > 159) and . != 58)' >/dev/null; then
		echo "Isolation state path resolves to a colon or control character." >&2
		return 1
	fi
	for path in "${ISOLATED_DIRECTORIES[@]}"; do
		target=$PISSBOX_WORKSPACE_DIR
		remaining=$path
		while [[ -n $remaining ]]; do
			part=${remaining%%/*}
			target="$target/$part"
			if [[ -L $target || ( -e $target && ! -d $target ) ]]; then
				echo "Isolated paths must contain only directories, never files or symlinks: $target" >&2
				return 1
			fi
			if [[ $remaining == */* ]]; then
				remaining=${remaining#*/}
			else
				remaining=""
			fi
		done
	done
}

prepare_isolated_creation() {
	ISOLATION_DIR=$(mktemp -d "$PISSBOX_STATE_DIR/isolation.XXXXXXXX") || return $?
	SBX_ARGS[0]=create
	SBX_ARGS+=(--env "PISSBOX_ISOLATION_DIR=$ISOLATION_DIR")
}

finish_isolated_creation() {
	local path target backing index=0
	if [[ ! -f $ISOLATION_DIR/name ]]; then
		echo "Isolation creation returned no identity; refusing to discover or guess a sandbox name." >&2
		return 1
	fi
	SANDBOX_NAME=$(< "$ISOLATION_DIR/name") || return $?
	if [[ ! $SANDBOX_NAME =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]]; then
		echo "Isolation creation returned an invalid identity; refusing sandbox actions." >&2
		return 1
	fi
	echo "Preparing isolated directories for '$SANDBOX_NAME'; failures require a new sandbox or explicit deletion, not setup replay." >&2
	# Recheck after creation before creating mountpoints on the live host.
	validate_isolated_paths "$PISSBOX_WORKSPACE_DIR" || return $?
	for path in "${ISOLATED_DIRECTORIES[@]}"; do
		index=$((index + 1))
		target="$PISSBOX_WORKSPACE_DIR/$path"
		backing="$ISOLATION_DIR/$index"
		mkdir -p "$target" || return $?
		mkdir "$backing" || return $?
		printf '%s\n' "$backing" > "$backing/.pissbox-isolation-mount" || return $?
		sbx mount "$SANDBOX_NAME" "$backing:$target" || return $?
		# Never write through an unverified mount: that could touch host dependencies.
		sbx exec --user 1000 "$SANDBOX_NAME" sh -eu -c '
			test ! -L "$1/.pissbox-isolation-mount"
			test "$(cat "$1/.pissbox-isolation-mount")" = "$2"
			probe=$(mktemp "$1/.pissbox-write.XXXXXXXX")
			rm "$probe"
		' isolation-mount "$target" "$backing" || return $?
	done
	sbx exec --user 1000 "$SANDBOX_NAME" bash -c \
		'exec bash "$HOME/.pissbox/scripts/project-setup.sh" --finalize-isolation' || return $?
}
