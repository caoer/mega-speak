#!/bin/sh
# scripts/build-linux.sh vX.Y.Z <out-dir> — the friends' Linux mega-speak, this host's arch.
#
# Runs from the repo root on a Linux host with docker (x86_64 or aarch64). Builds the CLI
# with PyInstaller (packaging/pyinstaller-linux.spec) inside AlmaLinux 8 (glibc 2.28) with a
# uv-managed Python 3.13, and writes to <out-dir>:
#   mega-speak-X.Y.Z-linux-<arch>.tar.gz   mega-speak/{bin/mega-speak,bin/_internal,Resources/defaults.toml}
#   mega-speak-X.Y.Z-linux-<arch>.glibc    the newest GLIBC_ symbol version the bundle needs
# Resources/defaults.toml points the CLI at our voice service, as in the Mac app.
# scripts/release-linux.sh runs this over ssh on one host per arch.
set -eu

[ $# -eq 2 ] && printf '%s' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || {
	sed -n '2p' "$0" | sed 's/^# //' >&2
	exit 2
}
ver=$1 short=${1#v} out=$2
API_URL=https://volc.0xtau.com/api/v3/plan/tts/unidirectional
IMAGE=mega-speak-linux-build:1
DOCKER=${DOCKER:-docker} # "sudo docker" where the user is not in the docker group
case $(uname -m) in
x86_64) arch=x86_64 ;;
aarch64 | arm64) arch=aarch64 ;;
*) echo "build-linux: no build for $(uname -m)" >&2; exit 2 ;;
esac
[ -f packaging/pyinstaller-linux.spec ] || { echo "build-linux: run from the repo root" >&2; exit 2; }
mkdir -p "$out"
out=$(cd "$out" && pwd)

$DOCKER build -q -t "$IMAGE" - >/dev/null <<'EOF'
FROM almalinux:8
COPY --from=ghcr.io/astral-sh/uv:0.9 /uv /usr/local/bin/uv
RUN dnf install -y -q binutils tar gzip && dnf clean all \
 && uv python install 3.13 && uv venv /opt/py --python 3.13 \
 && uv pip install --python /opt/py/bin/python 'pyinstaller==6.*'
EOF

name=mega-speak-$short-linux-$arch
$DOCKER run --rm -i -v "$PWD:/src:ro" -v "$out:/out" -e ver="$ver" -e name="$name" -e API_URL="$API_URL" \
	-e uid="$(id -u)" -e gid="$(id -g)" "$IMAGE" sh -eu <<'EOF'
w=/tmp/w && mkdir -p $w/py $w/pkg/mega-speak/Resources
sed "s/@version@/$ver/" /src/bin/mega-speak >$w/py/mega_speak.py
cd $w && MEGA_SPEAK_PY=$w/py /opt/py/bin/python -m PyInstaller --log-level WARN --noconfirm \
	--distpath $w/pkg/mega-speak --workpath $w/build /src/packaging/pyinstaller-linux.spec
cat >$w/pkg/mega-speak/Resources/defaults.toml <<TOML
# mega-speak $ver for Linux: the friends' build speaks through our voice service.
# The key comes from the friend's page line (mega-speak --set-key, ~/.config/mega-speak/api-key).
api_url = "$API_URL"
TOML
$w/pkg/mega-speak/bin/mega-speak --version
find $w/pkg -type f \( -name '*.so*' -o -path '*/bin/mega-speak' \) -exec objdump -T {} + 2>/dev/null \
	| grep -o 'GLIBC_[0-9.]*' | sed 's/GLIBC_//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1 >/out/$name.glibc
tar -C $w/pkg -czf /out/$name.tar.gz --owner=0 --group=0 mega-speak
chown "$uid:$gid" /out/$name.tar.gz /out/$name.glibc
EOF
echo "build-linux: $out/$name.tar.gz (glibc $(cat "$out/$name.glibc"))"
