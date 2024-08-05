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
	lastX, lastY int16
	container    xproto.Window
	target       xproto.Window
	drag         struct {
		active         bool
		moved          bool
		startX, startY int16
		x, y           int16
	}
	size struct {
		active         bool
		startX, startY int16
		startW, startH uint16
		x, y           int16
	}
}

func (w *Window) onPress(button int, px, py int16, time uint32) {
	if button == 1 {
		w.raise()
		geo, err := xproto.GetGeometry(x, xproto.Drawable(w.container))
		if err != nil {
			return
		}
		w.drag.active = true
		w.drag.moved = false
		w.drag.x = px
		w.drag.y = py
		w.drag.startX = geo.X
		w.drag.startY = geo.Y
	} else if button == 3 {
		geo, err := xproto.GetGeometry(x, xproto.Drawable(w.container))
		if err != nil {
			return
		}
		w.size.active = true
		w.size.x = px
		w.size.y = py
		w.size.startW = geo.Width
		w.size.startH = geo.Height
	}
}

func (w *Window) onRelease(button int, px, py int16, time uint32) {
	if button == 1 {
		w.drag.active = false
	} else if button == 3 {
		w.size.active = false
	}
}

func (w *Window) onMotion(x, y int16, time uint32) {
	if w.drag.active {
		w.drag.moved = true
		dx := x - w.drag.x
		dy := y - w.drag.y
		w.drag.startX += dx
		w.drag.startY += dy
		w.move(w.drag.startX, w.drag.startY)
		w.drag.x = x
		w.drag.y = y
	} else if w.size.active {
		dx := x - w.size.x
		dy := y - w.size.y
		w.size.startW = uint16(int16(w.size.startW) + dx)
		w.size.startH = uint16(int16(w.size.startH) + dy)
		w.resize(w.size.startW, w.size.startH)
		w.size.x = x
		w.size.y = y
	}
}

func (w *Window) raise() {
	xproto.ConfigureWindowUnchecked(x, w.container, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
}

func (w *Window) move(px, py int16) {
	xproto.ConfigureWindowUnchecked(x, w.container, xproto.ConfigWindowX|xproto.ConfigWindowY, []uint32{uint32(px), uint32(py)})
	w.lastX = px
	w.lastY = py
}

func (w *Window) resize(pw, ph uint16) {
	xproto.ConfigureWindowUnchecked(x, w.container, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{uint32(pw), uint32(ph)})
	xproto.ConfigureWindowUnchecked(x, w.target, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{uint32(pw), uint32(ph)})
}

func CreateWindowWrapper(target xproto.Window) (*Window, error) {
	wid := xproto.NewWindowID(x)

	geo, err := xproto.GetGeometry(x, xproto.Drawable(target))
	if err != nil {
		return nil, fmt.Errorf("failed to get geometry of window %d: %v", target, err)
	}

	err = xproto.CreateWindow(x, screen.RootDepth, wid, root, geo.X, geo.Y, geo.Width, geo.Height, WindowBorderWidth, xproto.WindowClassCopyFromParent, screen.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskButton1Motion | xproto.EventMaskButton3Motion | xproto.EventMaskStructureNotify | xproto.EventMaskSubstructureNotify | xproto.EventMaskFocusChange})
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %v", err)
	}

	err = xproto.ChangeWindowAttributes(x, wid, xproto.CwBorderPixel, []uint32{WindowBorderColor})

	return &Window{container: wid, target: target, lastX: geo.X, lastY: geo.Y}, nil
}
