#!/bin/sh
set -eu
if [ "$(id -u)" = 0 ]; then
  printf '%s\n' 'Run this installer as your normal desktop user, not root.' >&2
  exit 1
fi
source_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
destination="$HOME/.local/opt/care-anywhere"
applications="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
if [ -e "$destination" ]; then
  printf '%s\n' "CARE Anywhere is already installed at $destination. Stop CARE and move the old app folder aside before installing this alpha. Clinic data will be kept." >&2
  exit 1
fi
mkdir -p "$HOME/.local/opt" "$applications"
stage=$(mktemp -d "$HOME/.local/opt/.care-anywhere-XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
cp -a "$source_dir/." "$stage/"
mv "$stage" "$destination"
# Desktop-entry quoting: reject characters that require shell/Exec expansion.
case "$destination" in *'"'* | *'`'* | *'$'* | *'%'* | *'\'*)
  printf '%s\n' 'Home path cannot be represented safely in a desktop entry.' >&2
  exit 1
  ;;
esac
cat >"$applications/network.ohc.care-anywhere.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=CARE Anywhere
Comment=Your offline clinic, on this computer
Exec="$destination/care-anywhere"
Icon=$destination/care-anywhere.svg
Terminal=false
Categories=Office;MedicalSoftware;
StartupNotify=false
EOF
printf '%s\n' 'Installed. Open CARE Anywhere from your applications menu.'
