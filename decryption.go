package wiiudownloader

/*
#cgo CFLAGS: -I${SRCDIR}/cdecrypt
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR}
#cgo LDFLAGS: -lcdecrypt
#include <cdecrypt.h>
#include <ctype.h>
#include <stdlib.h>

// Declare a separate C function that calls the Go function progressCallback
extern void callProgressCallback(int progress);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

//export callProgressCallback
func callProgressCallback(progress C.int) {
	progressChan <- int(progress)
}

var progressChan = make(chan int)

func DecryptContents(path string, progress *ProgressWindow, deleteEncryptedContents bool) error {
	errorChan := make(chan error)
	defer close(errorChan)

	go runDecryption(path, errorChan, deleteEncryptedContents)

	progress.bar.SetText("Decrypting...")

	for progressInt := range progressChan {
		progress.bar.SetFraction(float64(progressInt) / 100)
	}

	if err := <-errorChan; err != nil {
		return err
	}

	return nil
}

func runDecryption(path string, errorChan chan<- error, deleteEncryptedContents bool) {
	argv := make([]*C.char, 2)
	argv[0] = C.CString("WiiUDownloader")
	argv[1] = C.CString(path)
	defer C.free(unsafe.Pointer(argv[0]))
	defer C.free(unsafe.Pointer(argv[1]))

	// Register the C callback function with C
	C.set_progress_callback(C.ProgressCallback(C.callProgressCallback))

	if int(C.cdecrypt_main(2, (**C.char)(unsafe.Pointer(&argv[0])))) != 0 {
		errorChan <- fmt.Errorf("decryption failed")
		return
	}

	if deleteEncryptedContents {
		doDeleteEncryptedContents(path)
	}

	close(progressChan) // Indicate the completion of the decryption process
	errorChan <- nil
}
