//go:build darwin

package mac

import (
	"math"
	"reflect"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/caoer/mega-speak/internal/i18n"
	"github.com/ebitengine/purego/objc"
)

// CGRect is a flat CGRect/NSRect: four doubles, passed in float registers.
type CGRect struct{ X, Y, W, H float64 }

// CGSize is a flat CGSize/NSSize.
type CGSize struct{ W, H float64 }

// Column is one column of a Board's table.
type Column struct {
	Title string
	Width float64 // points; a Wrap column takes the width the others leave
	Wrap  bool    // the text wraps onto as many lines as it needs; else one line, truncated
	Mono  bool    // monospaced digits
}

// Cell is one table cell: Text, and Sub under it in the secondary colour.
type Cell struct {
	Text, Sub, Tip string
	Dim            bool   // Text in the secondary colour
	Icon           string // an SF Symbol before Text, or alone when Text is ""
	Tint           string // Icon's colour, a Tint name
	SubTint        string // Sub's colour, a Tint name; "" the secondary label colour
}

// tint is a status colour by name: "red", "green", "orange", "blue", "gray".
// Each holds 3:1 on the table's light and dark rows: the system red and blue
// adapt; the system green and orange are too pale on white, so theirs are
// fixed shades that clear both.
func tint(name string) objc.ID {
	c := class("NSColor")
	switch name {
	case "red":
		return send(c, "systemRedColor")
	case "blue":
		return send(c, "systemBlueColor")
	case "green":
		return send(c, "colorWithSRGBRed:green:blue:alpha:", 0x24/255.0, 0x96/255.0, 0x4a/255.0, 1.0)
	case "orange":
		return send(c, "colorWithSRGBRed:green:blue:alpha:", 0xc8/255.0, 0x72/255.0, 0x0a/255.0, 1.0)
	case "gray":
		return send(c, "secondaryLabelColor")
	}
	return 0
}

// NSTextAlignment's right and centre swap places on arm64, which takes iOS's
// values.
var alignRight, alignCenter = func() (int, int) {
	if runtime.GOARCH == "amd64" {
		return 1, 2
	}
	return 2, 1
}()

const iconSize = 15.0 // an icon's box; the symbol draws at 12 pt inside it

// Board owns the menu bar menu and the Inbox/History window. Rows wrap their
// text, and optional row buttons archive or restore an entry. Callbacks run
// on the main thread.
type Board struct {
	OnOpen, OnClose                                      func()
	OnClick                                              func(row int)
	OnVoices                                             func()
	OnHistory, OnInbox, OnSettings, OnArchiveAll, OnQuit func()
	OnArchive                                            func(int)

	// main thread only
	item, window, table, note, empty, menu, archiveAll objc.ID
	cols                                               []Column
	widths                                             []float64
	rows                                               [][]Cell
	heights                                            []float64 // each row's, measured at Set
	meter                                              objc.ID   // a wrapping label that measures text
	noteText                                           string
	flashUntil                                         time.Time
}

const (
	boardHeader = 78.0 // the title line above the table
	cellPad     = 5.0  // above and below a cell's text
)

var (
	boardOnce   sync.Once
	boardTarget objc.ID
	theBoard    *Board // the one board: its target's methods reach it here
)

// boardClass is the Objective-C object the status button, the table and the
// window call: menu actions, table data/delegate and window lifecycle.
func boardClass() objc.ID {
	boardOnce.Do(func() {
		cls, err := objc.RegisterClass("MegaSpeakBoard", objc.GetClass("NSObject"),
			[]*objc.Protocol{objc.GetProtocol("NSTableViewDataSource"), objc.GetProtocol("NSTableViewDelegate"), objc.GetProtocol("NSWindowDelegate")}, nil,
			[]objc.MethodDef{
				{Cmd: sel("inbox:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnInbox != nil {
						theBoard.OnInbox()
					}
					theBoard.open()
				}},
				{Cmd: sel("history:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnHistory != nil {
						theBoard.OnHistory()
					}
					theBoard.open()
				}},
				{Cmd: sel("settings:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnSettings != nil {
						theBoard.OnSettings()
					}
				}},
				{Cmd: sel("archiveAll:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnArchiveAll != nil {
						theBoard.OnArchiveAll()
					}
				}},
				{Cmd: sel("archive:"), Fn: func(_ objc.ID, _ objc.SEL, button objc.ID) {
					if theBoard.OnArchive != nil {
						theBoard.OnArchive(sendInt(button, "tag"))
					}
				}},
				{Cmd: sel("quit:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnQuit != nil {
						theBoard.OnQuit()
					}
				}},
				{Cmd: sel("voices:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					if theBoard.OnVoices != nil {
						theBoard.OnVoices()
					}
				}},
				{Cmd: sel("rowClicked:"), Fn: func(_ objc.ID, _ objc.SEL, tv objc.ID) {
					if r := sendInt(tv, "clickedRow"); r >= 0 && r < len(theBoard.rows) && theBoard.OnClick != nil {
						theBoard.OnClick(r)
					}
				}},
				{Cmd: sel("numberOfRowsInTableView:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) int { return len(theBoard.rows) }},
				{Cmd: sel("tableView:viewForTableColumn:row:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, col objc.ID, row int) objc.ID {
					i, _ := strconv.Atoi(goString(send(col, "identifier")))
					return theBoard.cellView(row, i)
				}},
				// A CGFloat return: purego callbacks return no float, but a one-float
				// struct comes back in the same register (d0 on arm64).
				{Cmd: sel("tableView:heightOfRow:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, row int) struct{ H float64 } {
					if row < len(theBoard.heights) {
						return struct{ H float64 }{theBoard.heights[row]}
					}
					return struct{ H float64 }{24}
				}},
				{Cmd: sel("windowWillClose:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
					// closed by Esc: hand the focus back to the app before this one
					if app := send(class("NSApplication"), "sharedApplication"); sendBool(app, "isActive") && !anyWebWindowVisible() {
						send(app, "hide:", objc.ID(0))
					}
					if theBoard.OnClose != nil {
						theBoard.OnClose()
					}
				}},
			})
		if err != nil {
			panic("mac: " + err.Error())
		}
		boardTarget = send(send(objc.ID(cls), "alloc"), "init")
	})
	return boardTarget
}

// NewBoard puts the icon (an SF Symbol) in the menu bar with a w×h window
// titled title. There is one Board per process.
func NewBoard(symbol, title string, cols []Column, w, h float64) *Board {
	b := &Board{cols: cols}
	theBoard = b
	DoSync(func() { b.build(symbol, title, w, h) })
	return b
}

func (b *Board) build(symbol, title string, w, h float64) {
	t := boardClass()
	bar := send(class("NSStatusBar"), "systemStatusBar")
	b.item = send(send(bar, "statusItemWithLength:", -1.0), "retain") // NSVariableStatusItemLength
	btn := send(b.item, "button")
	img := send(class("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", nsString(symbol), nsString(title))
	send(img, "setTemplate:", true)
	send(btn, "setImage:", img)
	send(btn, "setToolTip:", nsString(title))
	send(btn, "setTarget:", t)
	b.menu = send(send(class("NSMenu"), "alloc"), "initWithTitle:", nsString(title))
	send(b.menu, "setAutoenablesItems:", false)
	for _, item := range []struct{ title, action, key string }{
		{i18n.T("menu.inbox"), "inbox:", ""}, {i18n.T("menu.history"), "history:", ""},
		{"", "", ""}, {i18n.T("menu.voices"), "voices:", ""}, {i18n.T("menu.settings"), "settings:", ","},
		{"", "", ""}, {i18n.T("menu.quit"), "quit:", "q"},
	} {
		if item.title == "" {
			send(b.menu, "addItem:", send(class("NSMenuItem"), "separatorItem"))
			continue
		}
		mi := send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:", nsString(item.title), sel(item.action), nsString(item.key))
		send(mi, "setTarget:", t)
		send(b.menu, "addItem:", mi)
	}
	send(b.item, "setMenu:", b.menu)

	root := send(send(send(class("NSView"), "alloc"), "initWithFrame:", CGRect{0, 0, w, h}), "autorelease")
	// An opaque ground in the table's colour: on the translucent popover
	// material the text's contrast would depend on the window behind it.
	ground := send(send(send(class("NSBox"), "alloc"), "initWithFrame:", CGRect{0, 0, w, h}), "autorelease")
	send(ground, "setBoxType:", 4) // NSBoxCustom
	send(ground, "setBorderWidth:", 0.0)
	send(ground, "setFillColor:", send(class("NSColor"), "controlBackgroundColor"))
	send(root, "addSubview:", ground)
	head := label(title, send(class("NSFont"), "systemFontOfSize:weight:", 13.0, 0.3), 0) // NSFontWeightSemibold
	send(head, "setFrame:", CGRect{16, h - 30, 110, 18})
	send(root, "addSubview:", head)
	voices := send(class("NSButton"), "buttonWithTitle:target:action:", nsString(i18n.T("menu.voices")), t, sel("voices:"))
	send(voices, "setFrame:", CGRect{130, h - 35, 106, 28})
	send(voices, "setToolTip:", nsString(i18n.T("menu.voices.tip")))
	send(root, "addSubview:", voices)
	b.note = label("", send(class("NSFont"), "systemFontOfSize:", 11.0), send(class("NSColor"), "secondaryLabelColor"))
	send(b.note, "setAlignment:", alignRight)
	send(b.note, "setLineBreakMode:", 4)
	send(b.note, "setFrame:", CGRect{16, h - 65, w - 166, 16})
	send(root, "addSubview:", b.note)

	for i, item := range []struct{ title, action string }{{i18n.T("menu.inbox"), "inbox:"}, {i18n.T("menu.history"), "history:"}, {i18n.T("menu.settings"), "settings:"}, {i18n.T("menu.archive_all"), "archiveAll:"}} {
		button := send(class("NSButton"), "buttonWithTitle:target:action:", nsString(item.title), t, sel(item.action))
		x := 244.0 + float64(i)*90
		send(button, "setFrame:", CGRect{x, h - 35, 88, 28})
		send(root, "addSubview:", button)
		if item.action == "archiveAll:" {
			b.archiveAll = button
		}
	}

	scroll := send(send(send(class("NSScrollView"), "alloc"), "initWithFrame:", CGRect{0, 0, w, h - boardHeader}), "autorelease")
	send(scroll, "setHasVerticalScroller:", true)
	send(scroll, "setAutohidesScrollers:", true)
	tv := send(send(class("NSTableView"), "alloc"), "initWithFrame:", CGRect{0, 0, w, h - boardHeader})
	send(tv, "setStyle:", 1) // NSTableViewStyleFullWidth
	send(tv, "setBackgroundColor:", send(class("NSColor"), "clearColor"))
	send(tv, "setGridStyleMask:", 2)           // NSTableViewSolidHorizontalGridLineMask
	send(tv, "setColumnAutoresizingStyle:", 0) // widths are fixed below
	send(tv, "setAllowsColumnReordering:", false)
	send(tv, "setAllowsColumnResizing:", false)
	send(tv, "setIntercellSpacing:", CGSize{12, 0})
	fixed, flex := 0.0, -1
	for i, c := range b.cols {
		if c.Wrap {
			flex = i
		} else {
			fixed += c.Width
		}
	}
	b.widths = make([]float64, len(b.cols))
	for i, c := range b.cols {
		b.widths[i] = c.Width
		if i == flex {
			b.widths[i] = w - fixed - 12*float64(len(b.cols)) - 16
		}
		col := send(send(send(class("NSTableColumn"), "alloc"), "initWithIdentifier:", nsString(strconv.Itoa(i))), "autorelease")
		send(col, "setTitle:", nsString(c.Title))
		send(col, "setWidth:", b.widths[i])
		send(col, "setResizingMask:", 0)
		send(tv, "addTableColumn:", col)
	}
	send(tv, "setDataSource:", t)
	send(tv, "setDelegate:", t)
	send(tv, "setTarget:", t)
	send(tv, "setAction:", sel("rowClicked:"))
	send(scroll, "setDocumentView:", tv)
	send(root, "addSubview:", scroll)
	b.table = tv

	b.meter = send(send(class("NSTextField"), "wrappingLabelWithString:", nsString("")), "retain")

	b.empty = send(class("NSTextField"), "wrappingLabelWithString:", nsString(""))
	send(b.empty, "setSelectable:", false)
	send(b.empty, "setAlignment:", alignCenter)
	send(b.empty, "setTextColor:", send(class("NSColor"), "secondaryLabelColor"))
	send(b.empty, "setFrame:", CGRect{40, (h-boardHeader)/2 - 20, w - 80, 40})
	send(b.empty, "setHidden:", true)
	send(root, "addSubview:", b.empty)

	b.window = send(send(class("NSWindow"), "alloc"), "initWithContentRect:styleMask:backing:defer:", CGRect{0, 0, w, h}, uint64(3), uint64(2), false)
	send(b.window, "setTitle:", nsString(i18n.T("window.inbox")))
	send(b.window, "setReleasedWhenClosed:", false)
	send(b.window, "setContentView:", root)
	send(b.window, "setDelegate:", t)
	send(b.window, "center")

}

// label is a one-line label in font, in fg when not 0.
func label(s string, font, fg objc.ID) objc.ID {
	l := send(class("NSTextField"), "labelWithString:", nsString(s))
	send(l, "setFont:", font)
	if fg != 0 {
		send(l, "setTextColor:", fg)
	}
	return l
}

// Set shows rows, the note beside the title and, with no rows, empty in
// their place. From any goroutine; on the main thread before it returns.
func (b *Board) Set(rows [][]Cell, note, empty string) {
	now(func() {
		b.noteText = note
		if time.Now().After(b.flashUntil) {
			send(b.note, "setStringValue:", nsString(note))
		}
		send(b.empty, "setStringValue:", nsString(empty))
		send(b.empty, "setHidden:", len(rows) > 0)
		if !reflect.DeepEqual(rows, b.rows) {
			b.rows = rows
			b.heights = make([]float64, len(rows))
			for i, r := range rows {
				b.heights[i] = b.rowHeight(r)
			}
			send(b.table, "reloadData")
		}
	})
}

// Flash shows s in place of the note for 4 s.
func (b *Board) Flash(s string) {
	now(func() {
		b.flashUntil = time.Now().Add(4 * time.Second)
		send(b.note, "setStringValue:", nsString(s))
		time.AfterFunc(4*time.Second, func() {
			Do(func() {
				if time.Now().After(b.flashUntil) {
					send(b.note, "setStringValue:", nsString(b.noteText))
				}
			})
		})
	})
}

// Open shows the Inbox/History window, frontmost.
func (b *Board) Open() { now(b.open) }

// Close closes the Inbox/History window.
func (b *Board) Close() { now(func() { send(b.window, "performClose:", objc.ID(0)) }) }

func (b *Board) open() {
	if b.OnOpen != nil {
		b.OnOpen()
	}
	send(send(class("NSApplication"), "sharedApplication"), "activateIgnoringOtherApps:", true)
	send(b.window, "makeKeyAndOrderFront:", objc.ID(0))
	send(b.table, "scrollRowToVisible:", 0)
}

// WindowNumber identifies the app-owned Inbox/History window.
func (b *Board) WindowNumber() int {
	var n int
	DoSync(func() { n = sendInt(b.window, "windowNumber") })
	return n
}

var (
	textFont, monoFont, subFont objc.ID
	fontsOnce                   sync.Once
)

func fonts() {
	fontsOnce.Do(func() {
		textFont = send(send(class("NSFont"), "systemFontOfSize:", 13.0), "retain")
		monoFont = send(send(class("NSFont"), "monospacedDigitSystemFontOfSize:weight:", 13.0, 0.0), "retain")
		subFont = send(send(class("NSFont"), "systemFontOfSize:", 11.0), "retain")
	})
}

// textHeight is the height of s in font: wrapped in a field width wide, as
// the field's cell draws it there, or one line when width is 0.
func (b *Board) textHeight(s string, font objc.ID, width float64) float64 {
	if width == 0 {
		s, width = "Ag", 1e4
	}
	send(b.meter, "setFont:", font)
	send(b.meter, "setStringValue:", nsString(s))
	return math.Ceil(objc.Send[CGSize](send(b.meter, "cell"), sel("cellSizeForBounds:"), CGRect{0, 0, width, 1e4}).H)
}

// cellHeights are a cell's text and second-line heights in column i.
func (b *Board) cellHeights(c Cell, i int) (main, sub float64) {
	fonts()
	font, w := textFont, 0.0
	if b.cols[i].Mono {
		font = monoFont
	}
	if b.cols[i].Wrap {
		w = b.widths[i]
		if c.Icon != "" {
			w -= iconSize + 4
		}
	}
	main = b.textHeight(c.Text, font, w)
	if c.Sub != "" {
		sub = b.textHeight("", subFont, 0)
	}
	return main, sub
}

// rowHeight is the tallest cell of a row, padded.
func (b *Board) rowHeight(r []Cell) float64 {
	h := 0.0
	for i, c := range r {
		if i >= len(b.cols) {
			break
		}
		main, sub := b.cellHeights(c, i)
		h = max(h, main+sub)
	}
	return h + 2*cellPad
}

// cellView is row's cell in column i: its text at the top, wrapped when the
// column wraps, and the second line under it.
func (b *Board) cellView(row, i int) objc.ID {
	if row >= len(b.rows) || i >= len(b.rows[row]) || i >= len(b.cols) {
		return 0
	}
	c, w, h := b.rows[row][i], b.widths[i], b.heights[row]
	v := send(send(send(class("NSView"), "alloc"), "initWithFrame:", CGRect{0, 0, w, h}), "autorelease")
	if c.Icon == "archive-action" || c.Icon == "restore-action" {
		title := i18n.T("menu.archive")
		if c.Icon == "restore-action" {
			title = i18n.T("menu.restore")
		}
		button := send(class("NSButton"), "buttonWithTitle:target:action:", nsString(title), boardTarget, sel("archive:"))
		send(button, "setFrame:", CGRect{0, (h - 28) / 2, w, 28})
		send(button, "setTag:", row)
		send(v, "addSubview:", button)
		return v
	}

	if c.Tip != "" {
		send(v, "setToolTip:", nsString(c.Tip))
	}
	fonts()
	font := textFont
	if b.cols[i].Mono {
		font = monoFont
	}
	fg := send(class("NSColor"), "labelColor")
	if c.Dim {
		fg = send(class("NSColor"), "secondaryLabelColor")
	}
	mainH, subH := b.cellHeights(c, i)
	top := h - cellPad - mainH // y up: the first line's bottom
	x := 0.0
	if c.Icon != "" {
		ix := 0.0
		if c.Text == "" {
			ix = (w - iconSize) / 2
		}
		send(v, "addSubview:", icon(c.Icon, c.Tint, CGRect{ix, top + (mainH-iconSize)/2, iconSize, iconSize}))
		x = iconSize + 4
	}
	if c.Text != "" {
		var main objc.ID
		if b.cols[i].Wrap {
			main = send(class("NSTextField"), "wrappingLabelWithString:", nsString(c.Text))
			send(main, "setSelectable:", false) // a click goes to the row
			send(main, "setFont:", font)
			send(main, "setTextColor:", fg)
		} else {
			main = oneLine(c.Text, font, fg)
		}
		send(main, "setFrame:", CGRect{x, top, w - x, mainH})
		send(v, "addSubview:", main)
	}
	if c.Sub != "" {
		sfg := tint(c.SubTint)
		if sfg == 0 {
			sfg = send(class("NSColor"), "secondaryLabelColor")
		}
		sub := oneLine(c.Sub, subFont, sfg)
		send(sub, "setFrame:", CGRect{x, top - subH, w - x, subH})
		send(v, "addSubview:", sub)
	}
	return v
}

// icon is an image view of an SF Symbol in a Tint colour.
func icon(symbol, tintName string, frame CGRect) objc.ID {
	img := send(class("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", nsString(symbol), nsString(symbol))
	cfg := send(class("NSImageSymbolConfiguration"), "configurationWithPointSize:weight:", 12.0, 0.23) // NSFontWeightMedium
	img = send(img, "imageWithSymbolConfiguration:", cfg)
	iv := send(class("NSImageView"), "imageViewWithImage:", img)
	if c := tint(tintName); c != 0 {
		send(iv, "setContentTintColor:", c)
	}
	send(iv, "setFrame:", frame)
	return iv
}

// oneLine is a label that truncates at its frame's edge.
func oneLine(s string, font, fg objc.ID) objc.ID {
	l := label(s, font, fg)
	send(l, "setLineBreakMode:", 4) // NSLineBreakByTruncatingTail
	return l
}

// ActivateBundle brings the running app with bundle id forward; false when
// none runs. Call it on the main thread.
func ActivateBundle(id string) bool {
	apps := send(class("NSRunningApplication"), "runningApplicationsWithBundleIdentifier:", nsString(id))
	if sendInt(apps, "count") == 0 {
		return false
	}
	send(send(apps, "firstObject"), "activateWithOptions:", uint(1<<1)) // NSApplicationActivateIgnoringOtherApps
	return true
}

func (b *Board) SetHistory(history bool) {
	now(func() {
		send(b.archiveAll, "setEnabled:", !history)
		if history {
			send(b.archiveAll, "setTitle:", nsString(i18n.T("menu.history_btn")))
			send(b.window, "setTitle:", nsString(i18n.T("window.history")))
		} else {
			send(b.archiveAll, "setTitle:", nsString(i18n.T("menu.archive_all")))
			send(b.window, "setTitle:", nsString(i18n.T("window.inbox")))
		}
	})
}
