# Mega Speak

Say something out loud from the command line with Volcengine's seed-tts-2.0 voices, with an emotion
that fits the message. It is built for coding agents that need to get a person's attention: an agent
runs `mega-speak --emotion urgent "CI 挂了，需要你看一眼。"` and the speaker says it.

- `mega-speak` — the CLI. It streams the audio, so speech starts about a second after the call.
- Mega Speak — a macOS menu bar app (a speech bubble) listing what was said, newest first, so a line
  you missed can be read. A click on a line switches [herdr](https://herdr.dev) to the agent that said it.
  **选择音色…** opens a voice library with search, favorites, descriptions, avatars, language/age/gender/category filters,
  and local official preview audio. **使用此音色** saves the default for future CLI and server calls.
- `mega-speak-server` — optional: lets other machines speak through this Mac's speaker.

## Install on a Mac

You need a Volcengine speech API key with seed-tts-2.0 enabled.

With nix:

```sh
nix profile install git+https://git.0xdao.app/caoer115/mega-speak#mega-speak
nix build git+https://git.0xdao.app/caoer115/mega-speak#mega-speak-app
./result/libexec/mega-speak/install-app.sh ./result/libexec/mega-speak/MegaSpeak.app
```

Without nix (needs python3 ≥ 3.11; Go for the menu bar app; `brew install ffmpeg` for streaming playback):

```sh
git clone https://git.0xdao.app/caoer115/mega-speak && cd mega-speak && sh scripts/install.sh
```

Then give it your key and try it without sound:

```sh
export MEGA_SPEAK_API_KEY=…          # or api_key_file / api_key_cmd in the config
mega-speak --dry-run "你好，我是 Mega Speak。"
mega-speak "你好，我是 Mega Speak。"
```

### The friends' build (Mega Speak.app)

`scripts/release-mac.sh vX.Y.Z` builds a self-contained `Mega Speak.app` for Macs with no Python or
Homebrew (Apple silicon, macOS 14+): the CLI and `mega-speak-server` built by PyInstaller
(`packaging/pyinstaller.spec`) sit beside the menu bar app in `Contents/MacOS`, and
`Contents/Resources/defaults.toml` points them at our voice service. It is signed with the 南山图
Developer ID, notarized and stapled, packed in a dmg, uploaded to the friends' page and announced in
its feed record `release.megaspeak-mac`; the app checks that feed at launch and daily and offers
**更新 / 跳过此版本 / 稍后**. Publishing goes through mega-update's `scripts/page-publish.sh` (retried, the feed put only after the
upload's sha256 matches; `PUBLISH_VIA=<ssh host>` publishes from another host). `--dry-run` builds and
signs but publishes nothing. The voice key reaches the app from the friend's page: the app owns the URL
scheme `megaspeak`, and opening `megaspeak://set-key?key=<url-encoded key>` stores the key as
`mega-speak --set-key` does (0600 `~/.config/mega-speak/api-key`), asks before replacing a different saved
key (showing both last fours), and leaves a key from `MEGA_SPEAK_API_KEY`, `api_key_file` or
`api_key_cmd` in force. On first launch with no key the settings window opens at the voice key, saying to
tap the page's Mega Speak button; a key can also be pasted there. `--key-state` says where the key comes
from, never the key. The app speaks Chinese when the first macOS preferred language is Chinese and English
otherwise (every line is in `internal/i18n`); the feed's `notes` are Chinese and `notes_en` English
(`NOTES` / `NOTES_EN` for `scripts/release-mac.sh`).
For agents to call it, put the bundled CLI on PATH:
`sudo ln -sf "/Applications/Mega Speak.app/Contents/MacOS/mega-speak" /usr/local/bin/mega-speak`.
The nix build sets no channel, so it never checks a feed.

### The friends' Linux build

`scripts/release-linux.sh vX.Y.Z` builds the CLI alone for Linux x86_64 and aarch64 (`--out` files, no
audio device needed): `scripts/build-linux.sh` runs PyInstaller (`packaging/pyinstaller-linux.spec`) in an
AlmaLinux 8 container on one docker host per arch (`X86_HOST`, `ARM_HOST`), giving a tarball that needs
glibc 2.17 or newer and no Python. It uploads both tarballs and `scripts/install-linux.sh` (filled with
their urls and sha256) to the friends' page and writes the feed record `release.megaspeak-linux`. The page
shows each friend one line that unlocks the page with that friend's own Linux token, runs the installer
(into `~/.local`, no root) and stores the friend's voice key with `--set-key`; running the line again is
the update. `node scripts/friends-linux-tokens.mjs <links-file>` gives each friend in the owner's links
file that token: a viewer token labelled `friend-<name>-linux`, stored on the page encrypted with the
friend's voice key so only that friend's page can read it. Run it again after adding a friend;
`--rotate --only <name>` replaces one friend's token.

## Configure

`~/.config/mega-speak/config.toml`; [config.example.toml](config.example.toml) explains every key:
the voice, default emotion, speed, volume, the API endpoint (standard speech key, or an Ark Agent Plan
key), and where the key comes from. `mega-speak --print-config` shows what is in effect.

The app saves only its selected voice to `~/.config/mega-speak/voice.toml`; it never rewrites
credentials or other config files. Speech preferences are saved separately in `~/.config/mega-speak/speech.toml`. Configuration order is system config, user config, the app's voice preference, speech preferences,
then `$MEGA_SPEAK_CONFIG`; command-line flags override them all. `MEGA_SPEAK_VOICE_CONFIG` selects a different
preference path for an isolated app or test. `mega-speak --voice-state` reports the effective voice without
loading a key or synthesizing; `mega-speak --set-voice ID` saves the preference without speaking.

## Voice library

The app bundles the 445 voices advertised by the Ark Doubao-语音合成-2.0 experience on 2026-10-08.
Names, descriptions, tags, avatars and all 445 official previews work offline. Previews are public
provider samples downloaded from their catalog URLs and encoded to mono 24 kHz MP3 at 64 kbps;
pressing **试听** plays that file, makes no TTS request and does not consume synthesis characters.
Only one preview plays at a time; closing the selector stops it. Avatars use 120-pixel thumbnails.
The embedded catalog records the source and sample URL; extracted files are checked against SHA-256
and installed as one complete version under `~/Library/Application Support/MegaSpeak/library/`.
The advertised catalog is not a claim that every voice has been synthesis-tested on every account.

Favorites are stored separately in `~/Library/Application Support/MegaSpeak/favorites.json`, survive app
and catalog upgrades, and start empty. They do not change provider-side favorites. A JSON array of
existing voice IDs can be merged without replacing current favorites:

```sh
mega-speak-bar --import-favorites /path/favorites.json
```

The selector shows read/write and playback failures and offers a retry when settings cannot be read.
`mega-speak-bar --show-voices` opens it directly. For isolated verification, `--settings-dir` selects the
favorites/cache directory and `--speak` selects the CLI executable.

## Use

```sh
mega-speak --emotion cheerful "好消息：PR 合进去了。"
mega-speak --instruction "你用像体育解说员一样激动的语气说" "测试全部通过！"
echo "长一点的文本" | mega-speak -
mega-speak --out clip.mp3 "…"      # a file, no sound
mega-speak --list-emotions
```

Every line played is appended to `~/.local/share/mega-speak/said.jsonl`, which the menu bar app reads.

## Another machine's speaker (optional)

Run `mega-speak-server <port> <token-file> <path-to-mega-speak>` on the Mac with the speaker. It listens
on loopback only; put a tunnel or reverse proxy in front of it. On the other machines set
`output_host`, `output_url` and `token_file` (config.example.toml). They need no Volcengine key.

## Voice instructions and context

The menu bar now opens a menu with **Inbox**, **History**, **选择音色…**, **设置…**, and **退出 Mega Speak**. In Settings, **播报偏好** is a free-form direction with editable presets for whispering, faster delivery, slower delivery and Sichuan dialect. Save applies to the next local or remotely requested utterance. Numeric speed and playback volume can also be saved; explicit caller flags override these numeric defaults. The local delivery direction is combined with the caller's instruction, with the local preference stated as the priority when they conflict.

```sh
mega-speak --instruction "你压低声音，用悄悄话的语气轻轻说。" "测试完成了。"
mega-speak --instruction "语速快一些，但吐字清楚。" --context "用户今天很忙，只需要简短汇报。" "部署已经完成。"
mega-speak --settings-state
mega-speak --set-settings '{"delivery_instruction":"用悄悄话轻声说。","speed":1.0,"volume":80}'
```

`--context` carries explicit dialogue context (up to 1000 characters), and `--instruction` carries a direction (up to 300 characters). Both forward through HTTP and SSH. `--section-id ID` optionally groups serial synthesis requests into one provider-side conversation; give each conversation a unique ID and do not share it across unrelated agents. Context and section IDs are not automatically inferred or retained in the playback log.

The API's `additions.context_texts` currently honors **only the first string**. Mega Speak combines explicit context, per-call direction and the user's local delivery preference into that one string, separate from the text to speak. The existing plan endpoint, `seed-tts-2.0` resource and JSON-encoded `additions` envelope remain unchanged. Directions are model-guided: their exact audible effect varies with the selected voice. See the official [V3 parameters](https://docs.volcengine.com/docs/DoubaoVoice/WebSocketBidirectionalStreaming-V3?lang=zh) and [2.0 context examples](https://docs.volcengine.com/docs/DoubaoVoice/SoundReplication20BestPractices?lang=zh).

## Inbox and app lifecycle

**归档** hides one utterance from Inbox. **全部归档** archives the currently listed Inbox entries. **History** retains every log entry, and **恢复** returns an archived entry to Inbox. Viewing a line does not automatically archive it. Archive decisions persist in `~/Library/Application Support/MegaSpeak/inbox.json`; archiving never modifies `said.jsonl`. New logs have UUIDs; older logs get stable identities based on their original content and byte position so repeated identical messages can be archived individually. Malformed archive state is reported instead of silently clearing the Inbox.

One app instance per user owns `~/.cache/mega-speak-app/instance.lock`. A subsequent launch asks the existing app to open a view over a local Unix socket, then exits without adding another icon. `mega-speak-bar --show-settings` opens settings directly. Isolated UI testing must also set `--instance-dir`, `--settings-dir`, `--log`, `MEGA_SPEAK_SETTINGS_CONFIG` and `MEGA_SPEAK_VOICE_CONFIG` to scratch paths. Intentional Quit unloads the launch agent only if its PID owns this app instance, avoiding a KeepAlive relaunch; normal launch or login starts the app again.
