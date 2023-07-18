package wiiudownloader

/*
#cgo CFLAGS: -I${SRCDIR}/cdecrypt
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR}
#cgo LDFLAGS: -lcdecrypt
#include <cdecrypt.h>
#include <ctype.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

var (
	wg              sync.WaitGroup
	decryptionDone  = false
	decryptionError = false
)

func decryptContents(path string, progress *int) error {
	wg.Add(1)
	go runDecryption(path, progress)
	for !decryptionDone {
		fmt.Println(*progress)
		time.Sleep(500 * time.Millisecond)
	}

	wg.Wait()

	if decryptionError {
		decryptionDone = false
		decryptionError = false
		return fmt.Errorf("decryption failed")
	}
	return nil
}

func runDecryption(path string, progress *int) {
	defer wg.Done()
	argv := make([]*C.char, 2)
	argv[0] = C.CString("WiiUDownloader")
	argv[1] = C.CString(path)
	if int(C.cdecrypt_main(2, (**C.char)(unsafe.Pointer(&argv[0])), (*C.int)(unsafe.Pointer(progress)))) != 0 {
		decryptionError = true
	}
	decryptionDone = true
}
