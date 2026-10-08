//go:build darwin

package mac

// CopyText runs on AppKit's thread and places one plain string on the pasteboard.
func CopyText(text string) bool {
	ok := false
	DoSync(func() {
		p := send(class("NSPasteboard"), "generalPasteboard")
		send(p, "clearContents")
		ok = sendBool(p, "setString:forType:", nsString(text), nsString("public.utf8-plain-text"))
	})
	return ok
}
