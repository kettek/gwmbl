package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type WindowAction int

const (
	WindowActionNone WindowAction = iota
	WindowActionMove
	WindowActionResize
)

const WindowBorderWidth = 4
const WindowBorderColor = 0x772277
const WindowBorderActiveColor = 0xFF00FF

func AddWindowBorder(x *xgb.Conn, w xproto.Window) error {
	xproto.ChangeWindowAttributesChecked(x, w, xproto.CwBorderPixel, []uint32{WindowBorderColor})
	return xproto.ConfigureWindowChecked(x, w, xproto.ConfigWindowBorderWidth, []uint32{WindowBorderWidth}).Check()
}

func RemoveWindowBorder(x *xgb.Conn, w xproto.Window) error {
	xproto.ChangeWindowAttributesChecked(x, w, xproto.CwBorderPixel, []uint32{0x000000})
	return xproto.ConfigureWindowChecked(x, w, xproto.ConfigWindowBorderWidth, []uint32{0}).Check()
}

func SetWindowBorderColor(x *xgb.Conn, w xproto.Window, color uint32) error {
	return xproto.ChangeWindowAttributesChecked(x, w, xproto.CwBorderPixel, []uint32{color}).Check()
}

func RaiseWindow(x *xgb.Conn, w xproto.Window) error {
	return xproto.ConfigureWindowChecked(x, w, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove}).Check()
}

func FocusedWindow(x *xgb.Conn) (xproto.Window, error) {
	if err := xproto.GrabServerChecked(x).Check(); err != nil {
		return 0, err
	}
	defer xproto.UngrabServer(x)

	active, err := xproto.GetInputFocus(x).Reply()
	if err != nil {
		return 0, err
	}

	window := active.Focus

	for {
		reply, err := xproto.QueryTree(x, window).Reply()
		if err != nil {
			return 0, err
		}
		if reply.Root == window || reply.Parent == reply.Root {
			break
		} else {
			window = reply.Parent
		}
	}

	return active.Focus, nil
}

func FocusWindow(x *xgb.Conn, w xproto.Window) error {
	return xproto.SetInputFocusChecked(x, xproto.InputFocusPointerRoot, w, xproto.TimeCurrentTime).Check()
}

func WindowGeometry(x *xgb.Conn, w xproto.Window) (int16, int16, uint16, uint16, error) {
	geo, err := xproto.GetGeometry(x, xproto.Drawable(w)).Reply()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return geo.X, geo.Y, geo.Width, geo.Height, nil
}

func RegisterToWindow(x *xgb.Conn, w xproto.Window) error {
	return xproto.ChangeWindowAttributes(x, w, xproto.CwEventMask, []uint32{xproto.EventMaskPointerMotion}).Check()
}

func UnregisterFromWindow(x *xgb.Conn, w xproto.Window) error {
	return xproto.ChangeWindowAttributes(x, w, xproto.CwEventMask, []uint32{xproto.EventMaskNoEvent}).Check()
}

func ParentWindow(x *xgb.Conn, w xproto.Window) error {
	// TODO: Create a new window, get the target window's geom, resize new window to fit window geom + our border, then reparent target window to new window.
	return nil
}

func UnparentWindow(x *xgb.Conn, w xproto.Window) error {
	// TODO: Reparent window to root, then destroy owning window.
	return nil
}

func Cleanup(x *xgb.Conn, root xproto.Window) {
	if tree, err := xproto.QueryTree(x, root).Reply(); err == nil {
		for _, child := range tree.Children {
			RemoveWindowBorder(x, child)
		}
	}
}

func main() {
	var x *xgb.Conn
	var err error
	if x, err = xgb.NewConn(); err != nil {
		panic(err)
	}

	setup := xproto.Setup(x)
	root := setup.DefaultScreen(x).Root

	if err := xproto.ChangeWindowAttributesChecked(x, root, xproto.CwEventMask, []uint32{xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskButton1Motion | xproto.EventMaskButton3Motion | xproto.EventMaskSubstructureNotify | xproto.EventMaskFocusChange}).Check(); err != nil {
		panic(err)
	}

	// Add borders to all our windows.
	if tree, err := xproto.QueryTree(x, root).Reply(); err == nil {
		for _, child := range tree.Children {
			if err := AddWindowBorder(x, child); err != nil {
				fmt.Println(err)
			}
		}
	}

	// Ensure we clean up our windows when we exit.
	defer func() {
		if recover() != nil {
			Cleanup(x, root)
		}
	}()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			Cleanup(x, root)
			os.Exit(0)
		}
	}()

	// Get the focused window and set its border color.
	focusedWindow, err := FocusedWindow(x)
	if err != nil {
		fmt.Println("couldn't get focused window", err)
	} else {
		SetWindowBorderColor(x, focusedWindow, WindowBorderActiveColor)
	}

	var window xproto.Window
	var windowX, windowY int16
	var windowWidth, windowHeight uint16
	var moved bool
	var startX, startY int16
	var action WindowAction
	for {
		var ev xgb.Event
		if ev, err = x.WaitForEvent(); err != nil {
			panic(err)
		}
		switch ev := ev.(type) {
		case xproto.ButtonPressEvent:
			// Get our window's starting X and Y.
			windowX, windowY, windowWidth, windowHeight, err = WindowGeometry(x, ev.Child)
			if err != nil {
				continue
			}
			startX = ev.RootX
			startY = ev.RootY
			window = ev.Child
			if ev.Detail == 1 {
				action = WindowActionMove
				moved = false
			} else if ev.Detail == 3 {
				action = WindowActionResize
			}
		case xproto.ButtonReleaseEvent:
			if !moved {
				SetWindowBorderColor(x, focusedWindow, WindowBorderColor)
				RaiseWindow(x, window)
				focusedWindow = window
				SetWindowBorderColor(x, focusedWindow, WindowBorderActiveColor)
				if err := FocusWindow(x, window); err != nil {
					fmt.Println("couldn't focus window", err)
				}
			}
			action = WindowActionNone
		case xproto.MotionNotifyEvent:
			if action == WindowActionMove {
				dx := ev.RootX - startX
				dy := ev.RootY - startY
				xproto.ConfigureWindow(x, window, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{uint32(windowX + dx), uint32(windowY + dy)})
				moved = true
			} else if action == WindowActionResize {
				dx := ev.RootX - startX
				dy := ev.RootY - startY
				xproto.ConfigureWindow(x, window, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{uint32(int16(windowWidth) + dx), uint32(int16(windowHeight) + dy)})
			}
		case xproto.CreateNotifyEvent:
			if err := AddWindowBorder(x, ev.Window); err != nil {
				fmt.Println(err)
			}
		case xproto.DestroyNotifyEvent:
			if err := RemoveWindowBorder(x, ev.Window); err != nil {
				fmt.Println(err)
			}
			if ev.Window == focusedWindow {
				// TODO: Set focused to last focused window.
			}
		case xproto.FocusInEvent:
			fmt.Println("focus in", ev.Event)
		case xproto.FocusOutEvent:
			fmt.Println("focus out", ev.Event)
		default:
			fmt.Println("unhandled", ev)
		}
	}
}
