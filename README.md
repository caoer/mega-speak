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

The app saves only its selected voice to `~/.config/mega-speak/voice.toml`; it never rewrites credentials,
emotion, speed or volume. Configuration order is system config, user config, the app's voice preference,
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
