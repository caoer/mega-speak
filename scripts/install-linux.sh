#!/bin/sh
# mega-speak @version@ for Linux — the installer behind the line on the friends' page.
#
# scripts/release-linux.sh fills in the version, the two builds and their sha256 and
# uploads this file to the page. The page's line unlocks the page into a cookie jar
# with the friend's own Linux token, pipes this file into sh with the jar in
# MEGA_JAR and the friend's voice key in MEGA_SPEAK_KEY, then deletes the jar.
# It installs for the current user, no root:
#   $PREFIX/lib/mega-speak/   the build (PREFIX = MEGA_SPEAK_PREFIX, default ~/.local)
#   $PREFIX/bin/mega-speak    a link to it
#   ~/.config/mega-speak/api-key   the voice key, 0600 (mega-speak --set-key)
# Running the line again installs the version the page offers then.
set -eu
HOST=https://page.sui.pics
VERSION=@version@
GLIBC_MIN=@glibc@

say() { printf '%s\n' "$*" >&2; }
die() { say "mega-speak: $*"; exit 1; }

[ -n "${MEGA_JAR:-}" ] && [ -f "$MEGA_JAR" ] || die "copy the whole line from the page and run it again / 请从页面复制完整的那一行重新运行"
[ "$(uname -s)" = Linux ] || die "this installer is for Linux / 这个安装只适用于 Linux"
case $(uname -m) in
x86_64 | amd64) url=@url_x86_64@ sum=@sha256_x86_64@ ;;
aarch64 | arm64) url=@url_aarch64@ sum=@sha256_aarch64@ ;;
*) die "no build for $(uname -m); there are builds for x86_64 and aarch64 / 没有 $(uname -m) 的版本，只有 x86_64 和 aarch64" ;;
esac

# glibc GLIBC_MIN or newer; musl (Alpine) cannot run the build.
libc=$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}') || libc=
[ -n "$libc" ] || die "needs glibc $GLIBC_MIN or newer (Alpine/musl is not supported) / 需要 glibc $GLIBC_MIN 或更新（不支持 Alpine/musl）"
[ "$(printf '%s\n%s\n' "$GLIBC_MIN" "$libc" | sort -t. -k1,1n -k2,2n | head -n 1)" = "$GLIBC_MIN" ] ||
	die "this system has glibc $libc; mega-speak needs $GLIBC_MIN or newer / 系统的 glibc 是 $libc，需要 $GLIBC_MIN 或更新"
for c in curl tar gzip; do command -v "$c" >/dev/null || die "needs $c / 需要先装 $c"; done
if command -v sha256sum >/dev/null; then sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null; then sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else die "needs sha256sum / 需要 sha256sum"; fi

prefix=${MEGA_SPEAK_PREFIX:-$HOME/.local}
lib=$prefix/lib/mega-speak
mkdir -p "$prefix/lib" "$prefix/bin"
tmp=$(mktemp -d "$prefix/lib/.mega-speak.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

say "mega-speak $VERSION: downloading / 下载中…"
curl -fsS -b "$MEGA_JAR" -o "$tmp/m.tar.gz" "$HOST$url" || die "download failed; run the line again / 下载失败，请重新运行那一行"
got=$(sha "$tmp/m.tar.gz")
[ "$got" = "$sum" ] || die "the download is damaged (sha256 $got); run the line again / 下载的文件不完整，请重新运行那一行"
tar -xzf "$tmp/m.tar.gz" -C "$tmp"
"$tmp/mega-speak/bin/mega-speak" --version >/dev/null || die "the build does not start on this system / 这个版本在这台机器上启动不了"

rm -rf "$lib.old"
[ ! -e "$lib" ] || mv "$lib" "$lib.old"
mv "$tmp/mega-speak" "$lib"
rm -rf "$lib.old"
ln -sf "$lib/bin/mega-speak" "$prefix/bin/mega-speak"

if [ -n "${MEGA_SPEAK_KEY:-}" ]; then
	printf '%s' "$MEGA_SPEAK_KEY" | "$prefix/bin/mega-speak" --set-key >/dev/null
fi
say "mega-speak $VERSION installed / 已安装: $prefix/bin/mega-speak"
say "voice key / 语音钥匙: $("$prefix/bin/mega-speak" --key-state)"
case ":$PATH:" in
*":$prefix/bin:"*) ;;
*) say "add it to PATH / 加入 PATH: echo 'export PATH=\"$prefix/bin:\$PATH\"' >> ~/.profile" ;;
esac
say "try / 试一下: mega-speak --out hello.mp3 \"你好\""
