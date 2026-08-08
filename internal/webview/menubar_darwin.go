// Menu-bar (NSStatusItem) support: a status-bar item with Open / Log in /
// Log out / Quit entries, driven through the Objective-C runtime with purego
// (CGO=0). Menu actions dispatch to a registered handler class whose IMPs call
// back into the Config callbacks, then reload the web view.
//
//go:build darwin

package webview

import (
	"sync"

	objc "github.com/go-macos/objc"
)

var (
	selSystemStatusBar  = objc.RegisterName("systemStatusBar")
	selStatusItemLength = objc.RegisterName("statusItemWithLength:")
	selButton           = objc.RegisterName("button")
	selInitMenuItem     = objc.RegisterName("initWithTitle:action:keyEquivalent:")
	selSetTargetSel     = objc.RegisterName("setTarget:")
	selAddItem          = objc.RegisterName("addItem:")
	selSeparatorItem    = objc.RegisterName("separatorItem")
	selSetMenu          = objc.RegisterName("setMenu:")
	selTerminate        = objc.RegisterName("terminate:")
	selReload           = objc.RegisterName("reload")
)

// NSVariableStatusItemLength.
const nsVariableStatusItemLength = -1.0

// menuState holds what the action IMPs need; there is a single menu per app.
var (
	menuOnce     sync.Once
	menuClass    objc.Class
	menuApp      objc.ID
	menuWin      objc.ID
	menuWebview  objc.ID
	menuOnLogin  func()
	menuOnLogout func()
	menuItemKeep objc.ID // retained handler instance
)

func registerMenuClass() {
	menuOnce.Do(func() {
		menuClass, _ = objc.RegisterClass("GoRedditMenu", objc.GetClass("NSObject"),
			[]objc.MethodDef{
				{Cmd: objc.RegisterName("onOpen:"), Fn: menuOnOpen},
				{Cmd: objc.RegisterName("onLogin:"), Fn: menuOnLoginAction},
				{Cmd: objc.RegisterName("onLogout:"), Fn: menuOnLogoutAction},
				{Cmd: objc.RegisterName("onQuit:"), Fn: menuOnQuit},
			})
	})
}

func menuOnOpen(_ objc.ID, _ objc.SEL, _ objc.ID) {
	menuApp.Send(selActivateIgnoringOtherApps, true)
	menuWin.Send(selMakeKeyAndOrderFront, objc.ID(0))
}

func menuOnLoginAction(_ objc.ID, _ objc.SEL, _ objc.ID) {
	if menuOnLogin != nil {
		menuOnLogin()
	}
	menuWebview.Send(selReload)
}

func menuOnLogoutAction(_ objc.ID, _ objc.SEL, _ objc.ID) {
	if menuOnLogout != nil {
		menuOnLogout()
	}
	menuWebview.Send(selReload)
}

func menuOnQuit(_ objc.ID, _ objc.SEL, _ objc.ID) {
	menuApp.Send(selTerminate, objc.ID(0))
}

// installMenuBar creates the status-bar item + menu. Called once, on the main
// thread, before the run loop starts.
func installMenuBar(app, win, webview objc.ID, cfg Config) {
	registerMenuClass()
	menuApp, menuWin, menuWebview = app, win, webview
	menuOnLogin, menuOnLogout = cfg.OnLogin, cfg.OnLogout

	handler := objc.ID(menuClass).Send(selAlloc).Send(selInit)
	handler.Send(selRetain)
	menuItemKeep = handler

	statusBar := objc.ID(objc.GetClass("NSStatusBar")).Send(selSystemStatusBar)
	item := statusBar.Send(selStatusItemLength, float64(nsVariableStatusItemLength))
	item.Send(selRetain)
	item.Send(selButton).Send(selSetTitle, nsString(cfg.MenuTitle))

	menu := objc.ID(objc.GetClass("NSMenu")).Send(selAlloc).Send(selInit)
	add := func(title, sel string) {
		mi := objc.ID(objc.GetClass("NSMenuItem")).Send(selAlloc).Send(
			selInitMenuItem, nsString(title), objc.RegisterName(sel), nsString(""))
		mi.Send(selSetTargetSel, handler)
		menu.Send(selAddItem, mi)
	}
	add("Open Reddit Reader", "onOpen:")
	add("Log in with Touch ID", "onLogin:")
	add("Log out", "onLogout:")
	menu.Send(selAddItem, objc.ID(objc.GetClass("NSMenuItem")).Send(selSeparatorItem))
	add("Quit Reddit Reader", "onQuit:")

	item.Send(selSetMenu, menu)
}
