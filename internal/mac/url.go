//go:build darwin

package mac

import "github.com/ebitengine/purego/objc"

// OnURL, when set before Run, receives every URL the system opens with this
// app (the kAEGetURL Apple event of a CFBundleURLTypes scheme), on the main
// thread. It is registered before the run loop starts, so the URL that
// launched the app arrives too.
var OnURL func(string)

const (
	aeGetURL        = 0x4755524c // 'GURL': kInternetEventClass and kAEGetURL
	keyDirectObject = 0x2d2d2d2d // '----'
)

func registerURLHandler() {
	cls, err := objc.RegisterClass("MegaSpeakURLHandler", objc.GetClass("NSObject"), nil, nil,
		[]objc.MethodDef{{Cmd: sel("handleURL:withReplyEvent:"), Fn: func(_ objc.ID, _ objc.SEL, event, _ objc.ID) {
			d := send(event, "paramDescriptorForKeyword:", uint32(keyDirectObject))
			if u := goString(send(d, "stringValue")); u != "" && OnURL != nil {
				OnURL(u)
			}
		}}})
	if err != nil {
		panic("mac: " + err.Error())
	}
	target := send(send(objc.ID(cls), "alloc"), "init")
	manager := send(class("NSAppleEventManager"), "sharedAppleEventManager")
	send(manager, "setEventHandler:andSelector:forEventClass:andEventID:", target, sel("handleURL:withReplyEvent:"), uint32(aeGetURL), uint32(aeGetURL))
}

// PreferredLanguage is the first of the user's macOS preferred languages, a
// BCP 47 tag such as "zh-Hans-CN", or "".
func PreferredLanguage() string {
	var tag string
	withPool(func() {
		tag = goString(send(send(class("NSLocale"), "preferredLanguages"), "firstObject"))
	})
	return tag
}
