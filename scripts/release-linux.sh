#!/usr/bin/env bash
# scripts/release-linux.sh [--dry-run] vX.Y.Z — the friends' staging mega-speak for Linux.
#
#   tag      HEAD carries vX.Y.Z and the tree is clean (a dry run builds HEAD at any
#            version and publishes nothing)
#   build    scripts/build-linux.sh on one docker host per arch, fed `git archive HEAD`
#            over ssh: mega-speak-X.Y.Z-linux-{x86_64,aarch64}.tar.gz and the glibc floor
#   publish  both tarballs uploaded to the friends' page and read back (sha256), then
#            scripts/install-linux.sh filled in with their urls and sums and uploaded,
#            then release.megaspeak-linux {version, url (the installer), sha256 (of the
#            installer), assets, glibc_min, notes, notes_en} put and read back
#
# The page builds each friend's line from that record: it unlocks the page with the
# friend's own Linux token (scripts/friends-linux-tokens.mjs) and runs the installer
# with the friend's voice key. Running the line again is the update.
#
# Env:
#   X86_HOST     ssh host that builds x86_64 (default workstation-nyc-2)
#   ARM_HOST     ssh host that builds aarch64 (default oci-us-phx-a1)
#   ARM_DOCKER   docker command on ARM_HOST (default "sudo docker"); X86_DOCKER likewise ("docker")
#   NOTES        release notes, plain Chinese; NOTES_EN the same in English
#   OUT          local work folder (default ./out.linux)
#   PAGE_BIN     ccc-pages tools (default ~/.local/share/ucc/shared/skills/ccc-pages/bin)
# Runs on a host holding the page owner's ucc identity; it builds nothing itself.
set -euo pipefail

SLUG=mega-friends-a020102a11bf95c8
KEY=release.megaspeak-linux

dry=false
[ "${1:-}" = --dry-run ] && { dry=true; shift; }
[ $# -eq 1 ] && [[ $1 =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || {
	sed -n '2p' "$0" | sed 's/^# //' >&2
	exit 2
}
ver=$1 short=${1#v}
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
out=${OUT:-$repo/out.linux}/$ver
bin=${PAGE_BIN:-$HOME/.local/share/ucc/shared/skills/ccc-pages/bin}
commit=$(git rev-parse --short HEAD)
if ! $dry; then
	[ "$(git describe --exact-match --tags HEAD 2>/dev/null)" = "$ver" ] || { echo "release-linux: HEAD is not tagged $ver" >&2; exit 2; }
	[ -z "$(git status --porcelain)" ] || { echo "release-linux: the tree has changes" >&2; exit 2; }
fi
echo "release-linux: $ver at $commit$($dry && echo ' (dry run)')"
rm -rf "$out" && mkdir -p "$out"

# --- build: one host per arch, in parallel -------------------------------------
build() { # arch host docker
	local dir=.cache/mega-speak-build/$ver
	git archive --format=tar HEAD | ssh "$2" "set -e; rm -rf $dir; mkdir -p $dir/src; tar -x -C $dir/src; cd $dir/src; DOCKER='$3' sh scripts/build-linux.sh $ver ../out"
	scp -q "$2:$dir/out/mega-speak-$short-linux-$1.*" "$out/"
}
build x86_64 "${X86_HOST:-workstation-nyc-2}" "${X86_DOCKER:-docker}" >"$out/build-x86_64.log" 2>&1 & p1=$!
build aarch64 "${ARM_HOST:-oci-us-phx-a1}" "${ARM_DOCKER:-sudo docker}" >"$out/build-aarch64.log" 2>&1 & p2=$!
wait $p1 || { tail -n 20 "$out/build-x86_64.log" >&2; exit 1; }
wait $p2 || { tail -n 20 "$out/build-aarch64.log" >&2; exit 1; }

glibc=$(sort -t. -k1,1n -k2,2n "$out"/*.glibc | tail -n 1)
declare -A sum
for a in x86_64 aarch64; do
	t=$out/mega-speak-$short-linux-$a.tar.gz
	tar -tzf "$t" | grep -x 'mega-speak/bin/mega-speak' >/dev/null || { echo "release-linux: $t has no mega-speak/bin/mega-speak" >&2; exit 1; }
	sum[$a]=$(sha256sum "$t" | cut -d' ' -f1)
	echo "release-linux: $(basename "$t") sha256 ${sum[$a]}"
done
echo "release-linux: glibc $glibc or newer"

# --- publish ---------------------------------------------------------------------
upload() { # path name type → /f/ url, after the stored bytes hash to the local sha256
	local id
	id=$("$bin/page-file" upload "$SLUG" "$1" --name "$2" --type "$3" | jq -er '.id // .data.id')
	"$bin/page-file" get "$SLUG" "$id" -o "$out/.check" >/dev/null
	[ "$(sha256sum <"$out/.check" | cut -d' ' -f1)" = "$(sha256sum <"$1" | cut -d' ' -f1)" ] || { echo "release-linux: $2 stored bytes differ" >&2; exit 1; }
	rm -f "$out/.check"
	echo "/f/$SLUG/$id"
}
fill() { # x86 url, arm url → the installer
	sed -e "s|@version@|$short|; s|@glibc@|$glibc|" \
		-e "s|@url_x86_64@|$1|; s|@sha256_x86_64@|${sum[x86_64]}|" \
		-e "s|@url_aarch64@|$2|; s|@sha256_aarch64@|${sum[aarch64]}|" scripts/install-linux.sh >"$out/install-$short.sh"
	! grep -n '@[a-z0-9_]*@' "$out/install-$short.sh" || { echo "release-linux: unfilled placeholder" >&2; exit 1; }
}
if $dry; then
	fill /f/$SLUG/DRY-x86_64 /f/$SLUG/DRY-aarch64
	echo "dry run: installer $out/install-$short.sh; nothing uploaded, $KEY not written"
	exit 0
fi
u64=$(upload "$out/mega-speak-$short-linux-x86_64.tar.gz" "mega-speak-$short-linux-x86_64.tar.gz" application/gzip)
uarm=$(upload "$out/mega-speak-$short-linux-aarch64.tar.gz" "mega-speak-$short-linux-aarch64.tar.gz" application/gzip)
fill "$u64" "$uarm"
uins=$(upload "$out/install-$short.sh" "mega-speak-install-$short.sh" text/plain)
sins=$(sha256sum "$out/install-$short.sh" | cut -d' ' -f1)
data=$(jq -cn --arg v "$short" --arg u "$uins" --arg s "$sins" --arg g "$glibc" \
	--arg n "${NOTES:-}" --arg e "${NOTES_EN:-}" \
	--arg u1 "$u64" --arg s1 "${sum[x86_64]}" --arg u2 "$uarm" --arg s2 "${sum[aarch64]}" \
	'{version: $v, url: $u, sha256: $s, glibc_min: $g, notes: $n, notes_en: $e,
	  assets: {x86_64: {url: $u1, sha256: $s1}, aarch64: {url: $u2, sha256: $s2}}}')
"$bin/page-data" put "$SLUG" "$KEY" --schema release-feed@1 --data "$data" --notify none >/dev/null
[ "$("$bin/page-data" get "$SLUG" "$KEY" --data-only | jq -c .)" = "$data" ] || { echo "release-linux: $KEY did not read back" >&2; exit 1; }
echo "release-linux: published $ver: $data"
