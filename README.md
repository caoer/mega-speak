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
