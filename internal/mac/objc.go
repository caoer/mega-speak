//go:build darwin

// Package mac drives AppKit through the Objective-C runtime with purego — no
// cgo. AppKit objects are touched on the main thread only (Do, DoSync).
package mac

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

func dlopen(path string) uintptr {
	h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		panic("mac: " + err.Error())
	}
	return h
}

var (
	libObjC   = dlopen("/usr/lib/libobjc.A.dylib")
	libSystem = dlopen("/usr/lib/libSystem.B.dylib")
	_         = dlopen("/System/Library/Frameworks/AppKit.framework/AppKit")
)

var (
	poolPush func() uintptr
	poolPop  func(uintptr)
)

func init() {
	purego.RegisterLibFunc(&poolPush, libObjC, "objc_autoreleasePoolPush")
	purego.RegisterLibFunc(&poolPop, libObjC, "objc_autoreleasePoolPop")
}

// withPool runs fn inside an autorelease pool, on one OS thread.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := poolPush()
	defer poolPop(p)
	fn()
}

var sels sync.Map

func sel(name string) objc.SEL {
	if s, ok := sels.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	sels.Store(name, s)
	return s
}

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func send(id objc.ID, name string, args ...any) objc.ID { return id.Send(sel(name), args...) }

func sendBool(id objc.ID, name string, args ...any) bool {
	return objc.Send[bool](id, sel(name), args...)
}

func sendInt(id objc.ID, name string, args ...any) int {
	return objc.Send[int](id, sel(name), args...)
}

// nsString returns an autoreleased NSString.
func nsString(s string) objc.ID {
	return send(class("NSString"), "stringWithUTF8String:", s)
}

func goString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	return objc.Send[string](id, sel("UTF8String"))
}
