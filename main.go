package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"

	"codeberg.org/gruf/go-xgb"
	"codeberg.org/gruf/go-xgb/pkg/icccm"
	"codeberg.org/gruf/go-xgb/pkg/xprop"
	"codeberg.org/gruf/go-xgb/xproto"
)

var x *xgb.XConn
var xprops *xprop.XPropConn
var screen xproto.ScreenInfo
var root xproto.Window
var windows []*Window
var focusedWindow Window
var movingWindow xproto.Window

func cleanup() {
	for _, w := range windows {
		xproto.ReparentWindowUnchecked(x, w.target, root, w.lastX, w.lastY)
		xproto.DestroyWindowUnchecked(x, w.container)
	}
}

func eccmName(w xproto.Window) (string, error) {
	reply, err := xprops.GetPropName(w, "_NET_WM_NAME")
	if err != nil {
		return "", err
	}
	return xprop.PropValStr(reply)
}

type WindowType string

const (
	WindowTypeDesktop      WindowType = "_NET_WM_WINDOW_TYPE_DESKTOP"
	WindowTypeDock         WindowType = "_NET_WM_WINDOW_TYPE_DOCK"
	WindowTypeToolbar      WindowType = "_NET_WM_WINDOW_TYPE_TOOLBAR"
	WindowTypeMenu         WindowType = "_NET_WM_WINDOW_TYPE_MENU"
	WindowTypeUtility      WindowType = "_NET_WM_WINDOW_TYPE_UTILITY"
	WindowTypeSplash       WindowType = "_NET_WM_WINDOW_TYPE_SPLASH"
	WindowTypeDialog       WindowType = "_NET_WM_WINDOW_TYPE_DIALOG"
	WindowTypeDropdownMenu WindowType = "_NET_WM_WINDOW_TYPE_DROPDOWN_MENU"
	WindowTypePopupMenu    WindowType = "_NET_WM_WINDOW_TYPE_POPUP_MENU"
	WindowTypeTooltip      WindowType = "_NET_WM_WINDOW_TYPE_TOOLTIP"
	WindowTypeNotification WindowType = "_NET_WM_WINDOW_TYPE_NOTIFICATION"
	WindowTypeCombo        WindowType = "_NET_WM_WINDOW_TYPE_COMBO"
	WindowTypeDnd          WindowType = "_NET_WM_WINDOW_TYPE_DND"
	WindowTypeNormal       WindowType = "_NET_WM_WINDOW_TYPE_NORMAL"
)

func (w WindowType) String() string {
	switch w {
	case WindowTypeDesktop:
		return "Desktop"
	case WindowTypeDock:
		return "Dock"
	case WindowTypeToolbar:
		return "Toolbar"
	case WindowTypeMenu:
		return "Menu"
	case WindowTypeUtility:
		return "Utility"
	case WindowTypeSplash:
		return "Splash"
	case WindowTypeDialog:
		return "Dialog"
	case WindowTypeDropdownMenu:
		return "DropdownMenu"
	case WindowTypePopupMenu:
		return "PopupMenu"
	case WindowTypeTooltip:
		return "Tooltip"
	case WindowTypeNotification:
		return "Notification"
	case WindowTypeCombo:
		return "Combo"
	case WindowTypeDnd:
		return "Dnd"
	case WindowTypeNormal:
		return "Normal"
	}
	return string(w)
}

func getWindowTypes(w xproto.Window) []WindowType {
	reply, err := xprops.GetPropName(w, "_NET_WM_WINDOW_TYPE")
	if err != nil {
		return nil
	}

	atoms, err := xprop.PropValAtoms(xprops, reply)
	if err != nil {
		return nil
	}
	var types []WindowType
	for _, atom := range atoms {
		reply, err := xproto.GetAtomName(x, atom)
		if err != nil {
			continue
		}
		types = append(types, WindowType(reply.Name))
	}

	return types
}

func doesWindowHaveType(w xproto.Window, types ...WindowType) bool {
	wtypes := getWindowTypes(w)
	for _, wtype := range wtypes {
		for _, t := range types {
			if wtype == t {
				return true
			}
		}
	}
	return false
}

func getWindowName(w xproto.Window) (string, error) {
	name, err := eccmName(w)
	if err != nil {
		return name, nil
	}
	name, err = icccm.WmNameGet(xprops, w)
	if err == nil {
		return name, nil
	}
	return "", err
}

func getWindowClass(w xproto.Window) (string, string, error) {
	class, err := icccm.WmClassGet(xprops, w)
	if err != nil {
		return "", "", err
	}
	return class.Instance, class.Class, nil
}

func getWindowGroup(w xproto.Window) xproto.Window {
	hints, err := icccm.WmHintsGet(xprops, w)
	if err != nil {
		return 0
	}
	return hints.WindowGroup
}

func hasHints(w xproto.Window) bool {
	_, err := icccm.WmHintsGet(xprops, w)
	return err == nil
}

func getWindowHints(w xproto.Window) *icccm.Hints {
	hints, err := icccm.WmHintsGet(xprops, w)
	if err != nil {
		return nil
	}
	return hints
}

func isWindowTransient(w xproto.Window) (xproto.Window, bool) {
	win, err := icccm.WmTransientForGet(xprops, w)
	if err != nil {
		return 0, false
	}
	return win, true
}

func getWindow(w xproto.Window) *Window {
	for _, window := range windows {
		if window.container == w || window.target == w {
			return window
		}
	}
	return nil
}

func tryAdopt(w xproto.Window) bool {
	if _, _, err := getWindowClass(w); err != nil {
		return false
	}

	// FIXME: Is it correct to ignore transient windows?
	if _, ok := isWindowTransient(w); ok {
		return false
	}

	if doesWindowHaveType(w, WindowTypePopupMenu, WindowTypeDialog, WindowTypeDropdownMenu) {
		return false
	}

	for _, window := range windows {
		if window.target == w || window.container == w {
			return false
		}
	}

	window, err := CreateWindowWrapper(w)
	if err != nil {
		fmt.Println("failed to create container for window", w)
		return false
	}

	if err := xproto.MapWindow(x, window.container); err != nil {
		fmt.Println("failed to map container for window", w)
		return false
	}

	if err := xproto.ReparentWindow(x, w, window.container, 0, 0); err != nil {
		fmt.Println("failed to reparent window", w)
		xproto.DestroyWindow(x, window.container)
		return false
	}

	if err := xproto.GrabButton(x, true, window.container, xproto.EventMaskButtonPress, xproto.GrabModeSync, xproto.GrabModeAsync, root, xproto.CursorNone, xproto.ButtonIndex1, xproto.ButtonMaskAny); err != nil {
		fmt.Println("failed to grab button", err)
	}

	windows = append(windows, window)

	return true
}

func tryDisown(w xproto.Window, remap bool) bool {
	for i, window := range windows {
		if window.target == w {
			if remap {
				xproto.ReparentWindowUnchecked(x, window.target, root, 0, 0)
			}
			xproto.DestroyWindowUnchecked(x, window.container)
			windows = append(windows[:i], windows[i+1:]...)
			return true
		}
	}
	return false
}

func main() {
	var err error
	var buf []byte
	if x, buf, err = xgb.Dial(""); err != nil {
		panic(err)
	}

	setup, err := xproto.Setup(x, buf)
	if err != nil {
		panic(err)
	}

	screen = setup.Roots[0]

	root = setup.Roots[0].Root

	xprops = &xprop.XPropConn{XConn: x}

	// Add hints to our root window.
	nameAtom, _ := xprops.Atom("_NET_WM_NAME", false)
	windowTypeAtom, _ := xprops.Atom("_NET_WM_WINDOW_TYPE", false)
	var supportedAtoms = []xproto.Atom{
		nameAtom,
		windowTypeAtom,
	}

	supportedData := make([]uint8, len(supportedAtoms)*4)
	for i, atom := range supportedAtoms {
		into := supportedData[i*4:]
		binary.LittleEndian.PutUint32(into, uint32(atom))
	}

	if err := xprops.ChangePropName(root, 32, "_NET_SUPPORTED", "ATOM", supportedData); err != nil {
		panic(err)
	}

	// Add borders to all our windows.
	tree, err := xproto.QueryTree(x, root)
	if err != nil {
		panic(err)
	}
	for _, child := range tree.Children {
		tryAdopt(child)
	}

	// Ensure we clean up our windows when we exit.
	defer func() {
		cleanup()
		if r := recover(); r != nil {
			panic(r)
		}
	}()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			cleanup()
			os.Exit(0)
		}
	}()

	// Get structure eventies.

	if err := xproto.ChangeWindowAttributes(x, root, xproto.CwEventMask, []uint32{xproto.EventMaskSubstructureNotify | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskButtonMotion | xproto.EventMaskFocusChange | xproto.EventMaskEnterWindow | xproto.EventMaskLeaveWindow}); err != nil {
		panic(err)
	}

	for {
		ev, err := x.Recv()
		if err != nil {
			panic(err)
		}
		switch ev := ev.(type) {
		case *xproto.ButtonPressEvent:
			if w := getWindow(ev.Event); w != nil {
				xproto.SetInputFocus(x, xproto.InputFocusParent, w.target, xproto.TimeCurrentTime)
				if ev.Child != 0 { // Our contents were hit -- just raise the window.
					w.raise()
				} else { // Otherwise handle as a press!
					w.onPress(int(ev.Detail), ev.RootX, ev.RootY, uint32(ev.Time))
				}
			} else {
				xproto.SetInputFocus(x, xproto.InputFocusNone, ev.Event, xproto.TimeCurrentTime)
			}
			xproto.AllowEvents(x, xproto.AllowReplayPointer, ev.Time)
		case *xproto.ButtonReleaseEvent:
			if w := getWindow(ev.Event); w != nil {
				w.onRelease(int(ev.Detail), ev.RootX, ev.RootY, uint32(ev.Time))
			}
		case *xproto.MotionNotifyEvent:
			if w := getWindow(ev.Event); w != nil {
				w.onMotion(ev.RootX, ev.RootY, uint32(ev.Time))
			}
		case *xproto.FocusInEvent:
			fmt.Println("focus in", ev)
		case *xproto.FocusOutEvent:
			fmt.Println("focus out", ev)
		case *xproto.UnmapNotifyEvent:
			fmt.Println("unmap notify", ev)
		case *xproto.DestroyNotifyEvent:
			tryDisown(ev.Window, false)
		case *xproto.CreateNotifyEvent:
			fmt.Println("create notify", ev, ev.BorderWidth, ev.OverrideRedirect, ev.Width, ev.Height, ev.X, ev.Y)
		case *xproto.ConfigureRequestEvent:
			fmt.Println("configure request", ev)
		case *xproto.MapNotifyEvent:
			tryAdopt(ev.Window)
		case *xproto.ConfigureNotifyEvent:
			fmt.Println("configure notify", ev.Event, ev.Window, ev.Width, ev.Height, ev.X, ev.Y)
			if w := getWindow(ev.Window); w != nil {
				if ev.X != 0 || ev.Y != 0 {
					xproto.ConfigureWindowUnchecked(x, w.target, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{0, 0})
				}
			}
		case *xproto.ReparentNotifyEvent:
			fmt.Println("reparent notify", ev)
		case *xproto.ClientMessageEvent:
			fmt.Println("client message", ev)
			reply, err := xproto.GetAtomName(x, ev.Type)
			if err != nil {
				fmt.Println("unknown atom", ev.Type)
			}
			fmt.Println("client atom", reply.Name)
		case *xproto.EnterNotifyEvent:
			fmt.Println("enter notify", ev)
		case *xproto.LeaveNotifyEvent:
			fmt.Println("leave notify", ev)
		default:
			fmt.Println("unknown event", ev)
		}
	}
}
