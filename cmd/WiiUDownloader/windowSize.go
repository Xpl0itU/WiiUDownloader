package main

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	WINDOW_MONITOR_CHROME_WIDTH  = 32
	WINDOW_MONITOR_CHROME_HEIGHT = 96
)

func planWindowSize(defaultW, defaultH, availW, availH int) (int, int) {
	if availW <= 0 || availH <= 0 {
		return defaultW, defaultH
	}
	width := defaultW
	if fits := availW - WINDOW_MONITOR_CHROME_WIDTH; fits < width {
		width = fits
	}
	height := defaultH
	if fits := availH - WINDOW_MONITOR_CHROME_HEIGHT; fits < height {
		height = fits
	}
	if width < MIN_WINDOW_WIDTH {
		width = MIN_WINDOW_WIDTH
	}
	if height < MIN_WINDOW_HEIGHT {
		height = MIN_WINDOW_HEIGHT
	}
	return width, height
}

func nativeSurface(win *gtk.Window) *gtk.NativeSurface {
	if win == nil {
		return nil
	}
	return &win.Root.NativeSurface
}

func windowArea(win *gtk.Window) (width, height, scale int, ok bool) {
	if win == nil || !win.Realized() {
		return 0, 0, 0, false
	}
	native := nativeSurface(win)
	if native == nil {
		return 0, 0, 0, false
	}
	surface := native.Surface()
	if surface == nil {
		return 0, 0, 0, false
	}
	display := gdk.BaseSurface(surface).Display()
	if display == nil {
		return 0, 0, 0, false
	}
	monitor := display.MonitorAtSurface(surface)
	if monitor == nil {
		return 0, 0, 0, false
	}
	geometry := monitor.Geometry()
	if geometry == nil {
		return 0, 0, 0, false
	}
	return geometry.Width(), geometry.Height(), monitor.ScaleFactor(), true
}

func applyWindowSize(win *gtk.Window, availW, availH int) (width, height int, changed bool) {
	if win == nil {
		return 0, 0, false
	}
	defaultW, defaultH := win.DefaultSize()
	width, height = planWindowSize(defaultW, defaultH, availW, availH)
	if width == defaultW && height == defaultH {
		return width, height, false
	}
	win.SetDefaultSize(width, height)
	return width, height, true
}

func fitWindowToMonitor(win *gtk.Window) {
	areaW, areaH, _, ok := windowArea(win)
	if !ok {
		return
	}
	applyWindowSize(win, areaW, areaH)
}

// fitWindowToMonitorBeforeShow realizes the window, sizes it to its monitor and
// only then presents it, so a window that would not fit is never shown at its
// full size first.
func fitWindowToMonitorBeforeShow(win *gtk.Window) {
	if win == nil {
		return
	}
	win.Realize()
	fitWindowToMonitor(win)
	win.Present()
}
