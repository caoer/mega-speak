//go:build darwin

package mac

import (
	"sync"

	"github.com/caoer/mega-speak/internal/i18n"
	"github.com/ebitengine/purego/objc"
)

var (
	laterOnce   sync.Once
	laterTarget objc.ID
)

// Later runs fn on the main thread from a run loop source, outside the main
// dispatch queue, so fn may run a modal loop (Choose, Alert): one run from a
// Do block would hold the serial main queue, and every Do and DoSync with it,
// until it ends.
func Later(fn func()) {
	laterOnce.Do(func() {
		cls, err := objc.RegisterClass("MegaSpeakLater", objc.GetClass("NSObject"), nil, nil,
			[]objc.MethodDef{{Cmd: sel("run:"), Fn: func(_ objc.ID, _ objc.SEL, n objc.ID) {
				id := objc.Send[uintptr](n, sel("unsignedLongValue"))
				pendingMu.Lock()
				f := pending[id]
				delete(pending, id)
				pendingMu.Unlock()
				withPool(f)
			}}})
		if err != nil {
			panic("mac: " + err.Error())
		}
		laterTarget = send(send(objc.ID(cls), "alloc"), "init")
	})
	pendingMu.Lock()
	nextID++
	id := nextID
	pending[id] = fn
	pendingMu.Unlock()
	withPool(func() {
		n := send(class("NSNumber"), "numberWithUnsignedLong:", id)
		send(laterTarget, "performSelectorOnMainThread:withObject:waitUntilDone:", sel("run:"), n, false)
	})
}

// Choose asks a question with one button per choice, brought to the front,
// and returns the index of the one clicked; -1 when it ended another way.
// Return clicks the first, Esc the last. Call it on the main thread from a
// menu action or Later, never from a Do block.
func Choose(title, info string, buttons ...string) int {
	send(send(class("NSApplication"), "sharedApplication"), "activateIgnoringOtherApps:", true)
	alert := send(send(send(class("NSAlert"), "alloc"), "init"), "autorelease")
	send(alert, "setMessageText:", nsString(title))
	send(alert, "setInformativeText:", nsString(info))
	var last objc.ID
	for _, b := range buttons {
		last = send(alert, "addButtonWithTitle:", nsString(b))
	}
	if len(buttons) > 1 {
		send(last, "setKeyEquivalent:", nsString("\x1b"))
	}
	return choiceIndex(sendInt(alert, "runModal"), len(buttons))
}

// Alert shows a message with one OK button, brought to the front. Same
// threading rule as Choose.
func Alert(title, info string) {
	Choose(title, info, i18n.T("app.ok"))
}

// choiceIndex maps an NSAlert response (NSAlertFirstButtonReturn = 1000, …)
// to a button index, or -1.
func choiceIndex(resp, n int) int {
	if i := resp - 1000; i >= 0 && i < n {
		return i
	}
	return -1
}
