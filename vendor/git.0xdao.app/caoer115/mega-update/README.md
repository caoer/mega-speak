# mega-update

The macOS updater Mega Voice and Mega Speak share: it checks a release feed on a
private ccc-pages page, remembers the versions the user skipped, and installs an
update by download, sha256 check and rename-swap. No UI and no cgo — the app
shows the prompt (更新 / 跳过此版本 / 稍后) and relaunches itself.

```
import update "git.0xdao.app/caoer115/mega-update"
```

Mega Speak imports this module. Mega Voice (caoer115/mega-asr) carries the
same package in-tree at `internal/update`, because that repo's public mirror
builds with no private module: a change here is copied there with its tests
(`git archive <tag> -- '*.go' | tar -x -C <mega-asr>/internal/update`, then
check the copy for private names — the mirror's guard refuses them).

## Contract

- **Build-time values.** `Channel`, `FeedURL` and `Token` come from `-X` ldflags in
  the staging build. An empty `Channel` (ZT's dev build) makes `Check` return
  `nil, nil` and `Apply` return `ErrDisabled`, with no request ever made.
- **Feed.** `FeedURL` is `https://<host>/d/<slug>/release.<app>?raw=1`, a record
  whose data is `{version, url, sha256, notes, notes_en, min_macos}`: `notes` in
  Chinese, `notes_en` in English (optional; an app shows `notes` when it is
  empty, and an app older than the field reads `notes` alone). `url` may be relative
  (`/f/<slug>/<id>`). The updater stores the feed and its `ETag` in
  `StateDir/state.json` and sends `If-None-Match`, so an unchanged feed answers 304.
- **Grant.** `POST <host>/unlock {slug, token}` gets a grant cookie. It is held
  in memory only and sent with the feed and `/f/` requests. A 401 unlocks once
  more. A 401 after a fresh unlock, or an unlock refused with `token_*`, returns
  `ErrRevoked`. After that the updater makes no request until the app restarts.
- **Offer.** A release is offered only when its version is newer than `Current`
  (`vMAJOR.MINOR.PATCH`), is not on the skip list, and `min_macos` ≤ the running
  macOS. `Skip(v)` silences that one version; a newer one prompts again.
- **Overlap.** `Check` calls that overlap share one request. The app calls `Check`
  at launch and every `update.Interval` (24 h).
- **Apply.** The download goes to a staging folder beside the app, is checked
  against its sha256, then unpacked: zip with `ditto`, dmg with `hdiutil`
  attach and `ditto`. The swap is by rename. The old bundle moves to
  `BackupPath(app)` (`.<Name>.app.bak`, hidden) and stays one cycle. On any error
  the installed app is unchanged. The process keeps running its old image, and
  the caller relaunches it (Mega Voice: `restart --when-idle`).
- **Fetch.** `Fetch(ctx, ref, header)` GETs anything else on the release page
  with the same grant — a record (`"models.megavoice-mac?raw=1"`, relative to
  `FeedURL`) or a `/f/` file, with `Range` to resume. Same 401 and
  `ErrRevoked` rules; `ErrDisabled` with no `Channel`.
- **Errors.** `UserMessage(err)` gives the line to show: `ErrRevoked` →
  更新不可用，请从页面重新安装, `ErrChecksum` → 更新校验失败，下次再试,
  `ErrNotWritable` → 请从页面重新安装 (open the page). Any other error gives `""`:
  stay silent and try again next cycle.

## Example

```go
u, err := update.New(update.Config{
	Channel:  Channel,   // -X main.Channel=staging
	FeedURL:  FeedURL,   // -X main.FeedURL=https://…/d/<slug>/release.megavoice-mac?raw=1
	Token:    PageToken, // -X main.PageToken=…
	Current:  Version,   // -X main.Version=v0.1.0
	StateDir: filepath.Join(os.Getenv("HOME"), "Library/Application Support/MegaVoice/update"),
})
if err != nil {
	log.Fatal(err)
}
check := func() {
	rel, err := u.Check(ctx)
	if msg := update.UserMessage(err); msg != "" {
		flash(msg)
	}
	if rel == nil {
		return
	}
	switch ask(rel.Version, rel.Notes) { // NSAlert: 更新 / 跳过此版本 / 稍后
	case "update":
		err := u.Apply(ctx, rel, "/Applications/Mega Voice.app")
		if err != nil {
			flash(update.UserMessage(err)) // "" for a transient failure
			return
		}
		restartWhenIdle()
	case "skip":
		u.Skip(rel.Version)
	} // 稍后: nothing; the next check asks again
}
check()
for range time.Tick(update.Interval) {
	check()
}
```

Tests: `go test ./...` (a fake ccc-pages over TLS; `Config.Extract` stands in for
ditto/hdiutil off macOS).

## Signing a release

`scripts/sign-macos.sh` signs an `.app` with 南山图's Developer ID Application
identity (team WRSJUX5YF6), notarizes and staples it, and with `--dmg` also
builds, signs, notarizes and staples the dmg friends download. It runs on macOS
(zmax-1), takes the identity and the notary key from osfiles
`secrets/svc/nanshantu-asc.sops.yaml` by sops (or from the environment), and
rcodesign from nixpkgs when it is not on PATH.

```
scripts/sign-macos.sh -e packaging/megavoice.entitlements \
  --dmg out/MegaVoice-0.1.0.dmg --volname "Mega Voice" out/MegaVoice.app
```

Every Mach-O in the bundle (Contents/MacOS, Frameworks, PlugIns, dylibs) is
signed inside out with the hardened runtime; `-e FILE` gives the main
executable its entitlements, `-e Contents/Frameworks/x.dylib:FILE` one nested
binary. `--dmg-host <ssh host>` builds the dmg's image on another Mac (a
throwaway VM) when this one's hdiutil cannot mount, as on zmax-1 while its
DiskArbitration hangs; the image is checked there and signed, notarized and
stapled here. The header of the script has the rest.

## Publishing a release

`scripts/page-publish.sh` is the publish half both release scripts call: the
dmg goes up with `page-file upload` and its id is printed only once the stored
sha256 equals the local file's; the feed record is put with `page-data put`
and read back. Every call is retried while it gets no answer, so the feed is
put last and only over verified bytes. With `PUBLISH_VIA=<ssh host>` the calls
run on that host (it needs page-file, page-data, jq and the page owner's
identity) after the file is copied there and its sha256 checked: the route for
a Mac whose own uploads drop, as zmax-1's do.

```
scripts/page-publish.sh upload <slug> out/MegaVoice-0.1.2.dmg MegaVoice-0.1.2.dmg application/x-apple-diskimage
scripts/page-publish.sh put <slug> release.megavoice-mac release-feed@1 '{"version":"0.1.2",…}'
```
