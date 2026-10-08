#!/bin/sh
# install-app.sh <MegaSpeak.app> — install the menu bar app at
# ~/Applications/MegaSpeak.app and restart its launch agent if one runs.
#
#   identity  $MEGA_SPEAK_SIGN_IDENTITY, default "-" (ad hoc): the app asks
#             for no permission, so no stable identity is needed
#   stamp     the source path and the executable's checksum, kept in
#             Contents/Resources/source; the same stamp is a no-op
#   restart   re-register the existing launch agent after replacing its
#             ad-hoc signed bundle, refreshing macOS code requirements
set -eu
[ $# -eq 1 ] || { echo "usage: install-app.sh <MegaSpeak.app>" >&2; exit 2; }
src=${1%/}
name=$(basename "$src")
dest=$HOME/Applications/$name
plist() { /usr/libexec/PlistBuddy -c "Print :$1" "$src/Contents/Info.plist"; }
label=$(plist CFBundleIdentifier)
exe=$(plist CFBundleExecutable)
stamp="$src $(cksum <"$src/Contents/MacOS/$exe" | cut -d' ' -f1)"
if [ "$(cat "$dest/Contents/Resources/source" 2>/dev/null)" = "$stamp" ] && codesign --verify --strict "$dest" 2>/dev/null; then
	exit 0
fi
parent=$(dirname "$dest")
mkdir -p "$parent"
stage=$(mktemp -d "$parent/.${name%.app}.XXXXXX")
trap 'rm -rf "$stage"' EXIT
cp -R "$src" "$stage/$name"
chmod -R u+w "$stage/$name"
mkdir -p "$stage/$name/Contents/Resources"
printf '%s\n' "$stamp" >"$stage/$name/Contents/Resources/source"
codesign --force --sign "${MEGA_SPEAK_SIGN_IDENTITY:--}" --identifier "$label" "$stage/$name"
# Swap by rename: a running copy keeps its old, now unlinked image.
[ -e "$dest" ] && mv "$dest" "$stage/old"
mv "$stage/$name" "$dest"
echo "install-app: $dest ← $src"
if launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then
	agent=$HOME/Library/LaunchAgents/$label.plist
	if [ -f "$agent" ]; then
		# kickstart retains stale LWCR/BTM registration after an ad-hoc
		# signature changes, which can leave a job in EX_CONFIG/spawn failed.
		launchctl bootout "gui/$(id -u)/$label"
		launchctl bootstrap "gui/$(id -u)" "$agent"
		echo "install-app: re-registered $label"
	else
		launchctl kickstart -k "gui/$(id -u)/$label"
		echo "install-app: restarted $label"
	fi
fi
