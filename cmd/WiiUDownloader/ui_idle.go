package main

import (
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
)

// uiIdleAdd schedules f on the GTK main loop. It is safe to call from any
// goroutine; f itself always runs on the main thread.
func uiIdleAdd(f func()) uint {
	if f == nil {
		return 0
	}
	return uiIdleAddBool(func() bool {
		f()
		return false
	})
}

// uiIdleAddBool schedules f on the GTK main loop and repeats it for as long as
// it returns true.
func uiIdleAddBool(f func() bool) uint {
	if f == nil {
		return 0
	}
	return uint(glib.IdleAdd(f))
}
