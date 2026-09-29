#!/bin/sh
# install.sh — install Mega Speak without nix, from a checkout of this repo.
#
#   the CLI       bin/mega-speak → ~/.local/bin (needs python3 >= 3.11; ffplay
#                 from ffmpeg streams audio as it arrives, else afplay plays it
#                 once it is whole)
#   the config    config.example.toml → ~/.config/mega-speak/config.toml, when
#                 there is none yet
#   the app       with Go installed: MegaSpeak.app in ~/Applications and a
#                 launch agent that starts it at login
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
bin=$HOME/.local/bin
mkdir -p "$bin" "$HOME/.config/mega-speak"
sed "s/@version@/$(git -C "$here" rev-parse --short HEAD 2>/dev/null || echo local)/" "$here/bin/mega-speak" >"$bin/mega-speak"
chmod +x "$bin/mega-speak"
echo "installed $bin/mega-speak"
[ -e "$HOME/.config/mega-speak/config.toml" ] || {
	cp "$here/config.example.toml" "$HOME/.config/mega-speak/config.toml"
	echo "wrote ~/.config/mega-speak/config.toml: set your key there, or export MEGA_SPEAK_API_KEY"
}

command -v go >/dev/null || { echo "no Go: skipping the menu bar app"; exit 0; }
[ "$(uname)" = Darwin ] || exit 0
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
app=$stage/MegaSpeak.app
mkdir -p "$app/Contents/MacOS"
(cd "$here" && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=local" -o "$app/Contents/MacOS/mega-speak-bar" ./cmd/mega-speak-bar)
cp "$here/packaging/Info.plist" "$app/Contents/Info.plist"
sh "$here/scripts/install-app.sh" "$app"
label=app.0xdao.megaspeak
agent=$HOME/Library/LaunchAgents/$label.plist
cat >"$agent" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>$label</string>
	<key>ProgramArguments</key><array><string>$HOME/Applications/MegaSpeak.app/Contents/MacOS/mega-speak-bar</string></array>
	<key>EnvironmentVariables</key><dict><key>PATH</key><string>$PATH</string></dict>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
</dict>
</plist>
PLIST
launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$agent"
echo "Mega Speak is in the menu bar (a speech bubble); it starts at login"
