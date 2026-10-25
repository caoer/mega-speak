//go:build darwin

package mac

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

// AppKit wants the process's first thread; the main goroutine keeps it.
func init() { runtime.LockOSThread() }

var (
	dispatchAsyncF func(queue, ctx, work uintptr)
	pthreadMainNP  func() int32
	mainQueue      uintptr
	trampoline     uintptr

	pendingMu sync.Mutex
	pending   = map[uintptr]func(){}
	nextID    uintptr
)

func init() {
	purego.RegisterLibFunc(&dispatchAsyncF, libSystem, "dispatch_async_f")
	purego.RegisterLibFunc(&pthreadMainNP, libSystem, "pthread_main_np")
	q, err := purego.Dlsym(libSystem, "_dispatch_main_q")
	if err != nil {
		panic("mac: " + err.Error())
	}
	mainQueue = q
	// One callback for every dispatched closure: purego callbacks are never
	// freed and capped at 2000, so closures travel as map keys.
	trampoline = purego.NewCallback(func(ctx uintptr) {
		pendingMu.Lock()
		fn := pending[ctx]
		delete(pending, ctx)
		pendingMu.Unlock()
		withPool(fn)
	})
}

func onMain() bool { return pthreadMainNP() != 0 }

// Do runs fn on the main thread, asynchronously.
func Do(fn func()) {
	pendingMu.Lock()
	nextID++
	id := nextID
	pending[id] = fn
	pendingMu.Unlock()
	dispatchAsyncF(mainQueue, id, trampoline)
}

// DoSync runs fn on the main thread and waits for it.
func DoSync(fn func()) {
	if onMain() {
		fn()
		return
	}
	done := make(chan struct{})
	Do(func() { defer close(done); fn() })
	<-done
}

// now runs fn at once when on the main thread, else queues it there.
func now(fn func()) {
	if onMain() {
		fn()
		return
	}
	Do(fn)
}

// Run makes this process an accessory (no Dock icon) AppKit app, starts ready
// on its own goroutine and runs the main event loop; it never returns. Call it
// from main.
func Run(ready func()) {
	if !onMain() {
		panic("mac.Run: not on the main thread")
	}
	p := poolPush()
	app := send(class("NSApplication"), "sharedApplication")
	send(app, "setActivationPolicy:", 1) // NSApplicationActivationPolicyAccessory
	if OnURL != nil {
		registerURLHandler()
	}
	poolPop(p)
	go ready()
	send(app, "run")
}
