package main

// #cgo pkg-config: glib-2.0
// #include <glib.h>
// #include <stdlib.h>
//
// extern gboolean goIdleCallback(gpointer data);
// extern void goIdleDestroy(gpointer data);
//
// static inline guint go_idle_add_full(gpointer data) {
//   return g_idle_add_full(G_PRIORITY_DEFAULT_IDLE, goIdleCallback, data, goIdleDestroy);
// }
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

func uiIdleAdd(f func()) uint {
	if f == nil {
		return 0
	}
	return uiIdleAddBool(func() bool {
		f()
		return false
	})
}

func uiIdleAddBool(f func() bool) uint {
	if f == nil {
		return 0
	}
	handle := cgo.NewHandle(f)
	ptr := C.malloc(C.size_t(unsafe.Sizeof(uintptr(0))))
	if ptr == nil {
		handle.Delete()
		return 0
	}
	*(*uintptr)(ptr) = uintptr(handle)
	id := C.go_idle_add_full(C.gpointer(ptr))
	if id == 0 {
		handle.Delete()
		C.free(ptr)
	}
	return uint(id)
}

//export goIdleCallback
func goIdleCallback(data unsafe.Pointer) C.gboolean {
	if data == nil {
		return C.FALSE
	}
	handle := cgo.Handle(*(*uintptr)(data))
	fn, ok := handle.Value().(func() bool)
	if !ok {
		return C.FALSE
	}
	if fn() {
		return C.TRUE
	}
	return C.FALSE
}

//export goIdleDestroy
func goIdleDestroy(data unsafe.Pointer) {
	if data == nil {
		return
	}
	handle := cgo.Handle(*(*uintptr)(data))
	handle.Delete()
	C.free(data)
}
