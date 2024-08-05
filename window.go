package main

import (
	"fmt"

	"codeberg.org/gruf/go-xgb/xproto"
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

type Window struct {
	container xproto.Window
	target    xproto.Window
	drag      struct {
		active bool
		startX int
		startY int
	}
}

func (w *Window) onPress(button int, px int, py int, time uint32) {
	fmt.Println("presssed!", button, px, py, time)
	if button == 1 {
		/*eo, err := xproto.GetGeometry(x, xproto.Drawable(w.container))
		if err != nil {
			return
			}*/
		w.drag.active = true
		w.drag.startX = px
		w.drag.startY = py
	}
}

func (w *Window) onRelease(button int, x int, y int, time uint32) {
	if button == 1 {
		w.drag.active = false
	}
	fmt.Println("released!", button, x, y, time)
}

func (w *Window) onMotion(x, y int, time uint32) {
	if w.drag.active {
		w.moveBy(x-w.drag.startX, y-w.drag.startY)
		w.drag.startX = x
		w.drag.startY = y
	}
	fmt.Println("motion!", x, y, time)
}

func (w *Window) moveBy(px, py int) {
	geo, err := xproto.GetGeometry(x, xproto.Drawable(w.container))
	if err != nil {
		return
	}
	geo.X += int16(px)
	geo.Y += int16(py)

	xproto.ConfigureWindow(x, w.container, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{uint32(geo.X), uint32(geo.Y)})
}

func CreateWindowWrapper(target xproto.Window) (xproto.Window, error) {
	wid := xproto.NewWindowID(x)

	geo, err := xproto.GetGeometry(x, xproto.Drawable(target))
	if err != nil {
		return 0, fmt.Errorf("failed to get geometry of window %d: %v", target, err)
	}

	err = xproto.CreateWindow(x, screen.RootDepth, wid, root, geo.X, geo.Y, geo.Width, geo.Height, WindowBorderWidth, xproto.WindowClassCopyFromParent, screen.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskButton1Motion | xproto.EventMaskButton3Motion | xproto.EventMaskStructureNotify | xproto.EventMaskSubstructureNotify})
	if err != nil {
		return 0, fmt.Errorf("failed to create window: %v", err)
	}

	err = xproto.ChangeWindowAttributes(x, wid, xproto.CwBorderPixel, []uint32{WindowBorderColor})

	return wid, nil
}
