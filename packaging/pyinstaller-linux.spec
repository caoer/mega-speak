# PyInstaller spec: the mega-speak CLI alone, onedir, for Linux hosts with no Python 3.11+.
# scripts/build-linux.sh runs it inside an old-glibc container and lays the result out as
#   mega-speak/bin/mega-speak            the executable
#   mega-speak/bin/_internal/            Python and its libraries
#   mega-speak/Resources/defaults.toml   read first by the frozen CLI (BUNDLE_DEFAULTS)
# Run it by hand with
#   MEGA_SPEAK_PY=<dir holding mega_speak.py> pyinstaller packaging/pyinstaller-linux.spec
import os

src = os.environ["MEGA_SPEAK_PY"]
a = Analysis([os.path.join(src, "mega_speak.py")], excludes=["tkinter", "unittest", "pydoc", "test"], noarchive=False)
exe = EXE(PYZ(a.pure), a.scripts, exclude_binaries=True, name="mega-speak", console=True, strip=False, upx=False)
coll = COLLECT(exe, a.binaries, a.datas, strip=False, upx=False, name="bin")
