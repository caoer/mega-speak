# PyInstaller spec: the mega-speak CLI and mega-speak-server as one onedir build in a
# Mega Speak.app skeleton, for Macs with no Python. scripts/release-mac.sh runs it and
# then adds the menu bar app; run it by hand with
#   MEGA_SPEAK_PY=<dir holding mega_speak.py and mega_speak_server.py> pyinstaller packaging/pyinstaller.spec
#
# Onedir, not one-file: a one-file binary unpacks Python into a temp folder on every
# call (0.44 s against 0.05 s per utterance on an M1), and the unpacked libraries are
# not the ones the notarized bundle signed. BUNDLE puts the binaries in
# Contents/Frameworks and the data in Contents/Resources, where codesign expects them.
import os

src = os.environ["MEGA_SPEAK_PY"]
names = {"mega-speak": "mega_speak.py", "mega-speak-server": "mega_speak_server.py"}
exes, parts = [], []
for name, script in names.items():
    a = Analysis([os.path.join(src, script)], excludes=["tkinter", "unittest", "pydoc", "test"], noarchive=False)
    exes.append(EXE(PYZ(a.pure), a.scripts, exclude_binaries=True, name=name, console=True,
                    strip=False, upx=False, target_arch="arm64"))
    parts += [a.binaries, a.datas]
coll = COLLECT(*exes, *parts, strip=False, upx=False, name="mega-speak")
app = BUNDLE(coll, name="Mega Speak.app", bundle_identifier="app.0xdao.megaspeak")
