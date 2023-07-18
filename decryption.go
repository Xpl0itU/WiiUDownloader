package wiiudownloader

/*
#cgo CFLAGS: -I${SRCDIR}/cdecrypt
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}/cdecrypt
#cgo LDFLAGS: -L${SRCDIR}/cdecrypt
#cgo LDFLAGS: -lcdecrypt
#include <cdecrypt.h>
#include <ctype.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func decryptContents(path string) error {
	argv := make([]*C.char, 2)
	argv[0] = C.CString("WiiUDownloader")
	argv[1] = C.CString(path)
	if int(C.cdecrypt_main(2, (**C.char)(unsafe.Pointer(&argv[0])))) != 0 {
		return fmt.Errorf("decryption failed")
	}
	return nil
}
