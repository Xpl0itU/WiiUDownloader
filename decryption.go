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

func DecryptContents(path string, progress *ProgressWindow, deleteEncryptedContents bool) error {
	wg.Add(1)
	progressInt := 1
	go runDecryption(path, &progressInt, deleteEncryptedContents)
	progress.bar.SetText("Decrypting...")
	for !decryptionDone {
		progress.bar.SetFraction(float64(progressInt) / 100)
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

func runDecryption(path string, progress *int, deleteEncryptedContents bool) {
	defer wg.Done()
	argv := make([]*C.char, 2)
	argv[0] = C.CString("WiiUDownloader")
	argv[1] = C.CString(path)
	if int(C.cdecrypt_main(2, (**C.char)(unsafe.Pointer(&argv[0])), (*C.int)(unsafe.Pointer(progress)))) != 0 {
		decryptionError = true
	}
	if deleteEncryptedContents {
		doDeleteEncryptedContents(path)
	}
	decryptionDone = true
}
