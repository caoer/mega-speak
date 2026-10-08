//go:build darwin

package mac

import (
	"github.com/ebitengine/purego/objc"
	"sync"
)

var _ = dlopen("/System/Library/Frameworks/WebKit.framework/WebKit")

// VoiceWindow is an app-owned, local WebKit window. Its bridge accepts
// messages only from the main frame; the HTML's CSP allows local media only.
type VoiceWindow struct {
	OnMessage   func(string)
	window, web objc.ID
}

var voiceOnce sync.Once
var voiceTarget objc.ID
var voiceWindow *VoiceWindow

func voiceClass() objc.ID {
	voiceOnce.Do(func() {
		// WebKit protocols can be lazily registered. Initialize its public
		// configuration class before looking them up, and never pass a nil
		// protocol to class_addProtocol (the runtime dereferences it).
		send(class("WKWebViewConfiguration"), "new")
		protocols := []*objc.Protocol{}
		for _, name := range []string{"WKScriptMessageHandler", "NSWindowDelegate"} {
			if p := objc.GetProtocol(name); p != nil {
				protocols = append(protocols, p)
			}
		}
		cls, err := objc.RegisterClass("MegaSpeakVoiceWindow", objc.GetClass("NSObject"),
			protocols, nil,
			[]objc.MethodDef{
				{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, message objc.ID) {
					if !sendBool(send(message, "frameInfo"), "isMainFrame") {
						return
					}
					body := send(message, "body")
					if sendBool(body, "isKindOfClass:", class("NSString")) && voiceWindow.OnMessage != nil {
						voiceWindow.OnMessage(goString(body))
					}
				}},
				{Cmd: sel("windowWillClose:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					voiceWindow.Eval("window.stopPreview?.()")
				}},
			})
		if err != nil {
			panic("mac: " + err.Error())
		}
		voiceTarget = send(send(objc.ID(cls), "alloc"), "init")
	})
	return voiceTarget
}

func NewVoiceWindow() *VoiceWindow {
	w := &VoiceWindow{}
	voiceWindow = w
	DoSync(func() {
		target := voiceClass()
		w.window = send(send(class("NSWindow"), "alloc"), "initWithContentRect:styleMask:backing:defer:", CGRect{0, 0, 1000, 740}, uint64(15), uint64(2), false)
		send(w.window, "setTitle:", nsString("Mega Speak · 音色"))
		send(w.window, "setReleasedWhenClosed:", false)
		send(w.window, "setMinSize:", CGSize{700, 520})
		send(w.window, "setDelegate:", target)
		send(w.window, "center")
		controller := send(send(class("WKUserContentController"), "alloc"), "init")
		send(controller, "addScriptMessageHandler:name:", target, nsString("megaSpeak"))
		config := send(send(class("WKWebViewConfiguration"), "alloc"), "init")
		send(config, "setUserContentController:", controller)
		send(config, "setWebsiteDataStore:", send(class("WKWebsiteDataStore"), "nonPersistentDataStore"))
		w.web = send(send(class("WKWebView"), "alloc"), "initWithFrame:configuration:", CGRect{0, 0, 1000, 740}, config)
		send(w.web, "setAutoresizingMask:", uint64(18))
		send(w.window, "setContentView:", w.web)
	})
	return w
}

func (w *VoiceWindow) Load(path, directory string) {
	now(func() {
		send(w.web, "loadFileURL:allowingReadAccessToURL:", send(class("NSURL"), "fileURLWithPath:", nsString(path)), send(class("NSURL"), "fileURLWithPath:", nsString(directory)))
	})
}
func (w *VoiceWindow) HTML(html string) {
	now(func() { send(w.web, "loadHTMLString:baseURL:", nsString(html), objc.ID(0)) })
}
func (w *VoiceWindow) Open() {
	now(func() {
		send(send(class("NSApplication"), "sharedApplication"), "activateIgnoringOtherApps:", true)
		send(w.window, "makeKeyAndOrderFront:", objc.ID(0))
	})
}
func (w *VoiceWindow) Eval(script string) {
	now(func() { send(w.web, "evaluateJavaScript:completionHandler:", nsString(script), objc.ID(0)) })
}
