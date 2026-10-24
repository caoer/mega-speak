#!/bin/bash
# scripts/release-mac.sh [--dry-run] vX.Y.Z — the friends' staging Mega Speak.app.
#
#   tag      HEAD carries vX.Y.Z and the tree is clean (a dry run builds any
#            version from any tree and publishes nothing)
#   build    packaging/pyinstaller.spec: the mega-speak CLI and mega-speak-server
#            (onedir, Python inside) in a Mega Speak.app; the menu bar app beside
#            them with -X main.channel=staging, main.feedURL, main.pageToken;
#            Contents/Resources/defaults.toml points the CLI at our voice service
#   sign     $MEGA_SIGN (mega-update scripts/sign-macos.sh): 南山图 Developer ID,
#            hardened runtime, notarized and stapled, app and dmg
#   dmg      Mega-Speak-X.Y.Z.dmg with an Applications link, and its sha256
#   publish  $MEGA_PUBLISH (mega-update scripts/page-publish.sh): the dmg
#            uploaded and its stored sha256 checked, then release.megaspeak-mac
#            {version, url, sha256, notes, min_macos} put and read back, each
#            call retried over a line that drops; a dry run prints both
#
# Env:
#   MEGA_SPEAK_TOKEN_FILE  the page's app-staging viewer token (0600 file); baked
#                          into the menu bar app, never printed
#   MEGA_SIGN              the signer (default ../mega-update/scripts/sign-macos.sh)
#   MEGA_PUBLISH           the publisher (default page-publish.sh beside MEGA_SIGN)
#   PUBLISH_VIA            an ssh host to publish from, for a Mac whose own uploads
#                          to page.sui.pics drop (page-publish.sh says what it needs)
#   MEGA_SPEAK_PYTHON      a python with PyInstaller (default: a uv venv in $OUT)
#   OUT                    work folder (default ./out.noindex: Spotlight skips it,
#                          so LaunchServices never registers a staging build)
#   NOTES                  the feed's release notes, plain Chinese
# Runs on macOS arm64. ZT's own build (app.nix) sets none of the -X values and
# never polls.
set -euo pipefail

SLUG=mega-friends-a020102a11bf95c8
HOST=https://page.sui.pics
KEY=release.megaspeak-mac
API_URL=https://volc.0xtau.com/api/v3/plan/tts/unidirectional
MIN_MACOS=14.0

dry=false
[ "${1:-}" = --dry-run ] && { dry=true; shift; }
[ $# -eq 1 ] && [[ $1 =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || {
	sed -n '2,3p' "$0" | sed 's/^# //' >&2
	exit 2
}
ver=$1 short=${1#v}
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
out=${OUT:-$repo/out.noindex}
sign=${MEGA_SIGN:-$repo/../mega-update/scripts/sign-macos.sh}
token_file=${MEGA_SPEAK_TOKEN_FILE:?set MEGA_SPEAK_TOKEN_FILE to the app-staging token file}
[ -s "$token_file" ] || { echo "release-mac: $token_file is empty" >&2; exit 2; }
[ -f "$sign" ] || { echo "release-mac: no signer at $sign (set MEGA_SIGN)" >&2; exit 2; }
publish=${MEGA_PUBLISH:-$(dirname "$sign")/page-publish.sh}
[ -f "$publish" ] || { echo "release-mac: no publisher at $publish (set MEGA_PUBLISH)" >&2; exit 2; }

commit=$(git rev-parse --short HEAD)
if ! $dry; then
	[ "$(git describe --exact-match --tags HEAD 2>/dev/null)" = "$ver" ] || { echo "release-mac: HEAD is not tagged $ver" >&2; exit 2; }
	[ -z "$(git status --porcelain)" ] || { echo "release-mac: the tree has changes" >&2; exit 2; }
fi
echo "release-mac: $ver at $commit$($dry && echo ' (dry run)')"

mkdir -p "$out"
touch "$out/.metadata_never_index"
stage=$out/$ver
rm -rf "$stage"
mkdir -p "$stage/py"

# --- build: the CLI and server, Python inside --------------------------------
py=${MEGA_SPEAK_PYTHON:-}
if [ -z "$py" ]; then
	[ -x "$out/pyenv/bin/python" ] || uv venv -q --python 3.13 --managed-python "$out/pyenv"
	uv pip install -q --python "$out/pyenv/bin/python" 'pyinstaller==6.*'
	py=$out/pyenv/bin/python
fi
sed "s/@version@/$ver/" bin/mega-speak >"$stage/py/mega_speak.py"
sed "s/@version@/$ver/" bin/mega-speak-server >"$stage/py/mega_speak_server.py"
MEGA_SPEAK_PY=$stage/py "$py" -m PyInstaller --log-level WARN --noconfirm \
	--distpath "$stage/dist" --workpath "$stage/build" packaging/pyinstaller.spec
app=$stage/dist/Mega\ Speak.app
rm -rf "$app/Contents/_CodeSignature" "$app/Contents/Resources/icon-windowed.icns"

# --- build: the menu bar app, with the staging channel -----------------------
# The token goes in through a file read by the shell, never on a printed line.
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -trimpath -o "$app/Contents/MacOS/mega-speak-bar" -ldflags "-s -w \
	-X main.version=$ver -X main.channel=staging \
	-X main.feedURL=$HOST/d/$SLUG/$KEY?raw=1 -X main.pageToken=$(tr -d '[:space:]' <"$token_file")" ./cmd/mega-speak-bar
cp packaging/Info.plist "$app/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$short" "$app/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$(git rev-list --count HEAD)" "$app/Contents/Info.plist"
plutil -replace CFBundleName -string "Mega Speak" "$app/Contents/Info.plist"
cat >"$app/Contents/Resources/defaults.toml" <<EOF
# Mega Speak $ver ($commit): the friends' build speaks through our voice service.
# The key is the one typed into Mega Speak's settings (~/.config/mega-speak/api-key).
api_url = "$API_URL"
EOF
"$app/Contents/MacOS/mega-speak" --version
"$app/Contents/MacOS/mega-speak-bar" --version

# --- sign, notarize, staple; dmg ---------------------------------------------
dmg=$stage/Mega-Speak-$short.dmg
sh "$sign" --dmg "$dmg" --volname "Mega Speak" "$app"
sum=$(shasum -a 256 "$dmg" | cut -d' ' -f1)
echo "$sum  $(basename "$dmg")" >"$dmg.sha256"
echo "release-mac: $dmg sha256 $sum"

# --- publish -------------------------------------------------------------------
notes=${NOTES:-}
upload=(sh "$publish" upload "$SLUG" "$dmg" "$(basename "$dmg")" application/x-apple-diskimage)
if $dry; then
	echo "dry run, not run: ${upload[*]}"
	echo "dry run, not run: page-publish.sh put $SLUG $KEY release-feed@1 {version:$short, url:/f/$SLUG/<id>, sha256:$sum, notes, min_macos:$MIN_MACOS}"
	exit 0
fi
id=$("${upload[@]}")
data=$(jq -cn --arg v "$short" --arg u "/f/$SLUG/$id" --arg s "$sum" --arg n "$notes" --arg m "$MIN_MACOS" \
	'{version: $v, url: $u, sha256: $s, notes: $n, min_macos: $m}')
sh "$publish" put "$SLUG" "$KEY" release-feed@1 "$data"
echo "release-mac: published $ver: $data"
