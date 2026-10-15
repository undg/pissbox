#!/bin/sh
set -eu

source_dir=${PISSBOX_PI_DIR:?Set PISSBOX_PI_DIR to the mounted host Pi directory}
case "$source_dir" in
  /*) ;;
  *) echo "Pi config path must be absolute: $source_dir" >&2; exit 1 ;;
esac

target="$HOME/.pi/agent"
if [ ! -d "$source_dir" ]; then
  echo "Pi config mount is missing inside the sandbox: $source_dir" >&2
  exit 1
fi

if [ -L "$target" ]; then
  if [ "$(readlink "$target")" != "$source_dir" ]; then
    echo "Refusing to replace existing Pi config symlink: $target" >&2
    exit 1
  fi
elif [ -e "$target" ]; then
  echo "Refusing to replace existing Pi config path: $target" >&2
  exit 1
else
  mkdir -p "$HOME/.pi"
  ln -s "$source_dir" "$target"
fi
