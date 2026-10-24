# mega-update

The macOS updater Mega Voice and Mega Speak share: it checks a release feed on a
private ccc-pages page, remembers the versions the user skipped, and installs an
update by download, sha256 check and rename-swap. No UI and no cgo — the app
shows the prompt (更新 / 跳过此版本 / 稍后) and relaunches itself.

```
import update "git.0xdao.app/caoer115/mega-update"
```

## Contract

- **Build-time values.** `Channel`, `FeedURL` and `Token` come from `-X` ldflags in
  the staging build. An empty `Channel` (ZT's dev build) makes `Check` return
  `nil, nil` and `Apply` return `ErrDisabled`, with no request ever made.
- **Feed.** `FeedURL` is `https://<host>/d/<slug>/release.<app>?raw=1`, a record
  whose data is `{version, url, sha256, notes, min_macos}`. `url` may be relative
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
