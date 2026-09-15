//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func showNativeFatalMessage(title, body string) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	bodyPtr, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return
	}
	const mbOK, mbIconError, mbSetForeground = 0x0, 0x10, 0x10000
	user32 := syscall.NewLazyDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(titlePtr)),
		uintptr(unsafe.Pointer(bodyPtr)), mbOK|mbIconError|mbSetForeground)
}
