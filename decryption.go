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
	"time"
	"unsafe"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"golang.org/x/sync/errgroup"
)

//export callProgressCallback
func callProgressCallback(progress C.int) {
	progressChan <- int(progress)
}

var progressChan chan int

func DecryptContents(path string, progress *ProgressWindow, deleteEncryptedContents bool) error {
	progressChan = make(chan int)

	errGroup := errgroup.Group{}

	errGroup.Go(func() error {
		return runDecryption(path, deleteEncryptedContents)
	})

	glib.IdleAdd(func() {
		progress.bar.SetText("Decrypting...")
	})

	for progressInt := range progressChan {
		if progressInt > 0 {
			glib.IdleAdd(func() {
				progress.bar.SetFraction(float64(progressInt) / 100)
			})
			for gtk.EventsPending() {
				gtk.MainIteration()
			}
		}
		time.Sleep(time.Millisecond * 10)
	}

	return errGroup.Wait()
}

func runDecryption(path string, deleteEncryptedContents bool) error {
	defer close(progressChan)
	argv := make([]*C.char, 2)
	argv[0] = C.CString("WiiUDownloader")
	argv[1] = C.CString(path)
	defer C.free(unsafe.Pointer(argv[0]))
	defer C.free(unsafe.Pointer(argv[1]))

	// Register the C callback function with C
	C.set_progress_callback(C.ProgressCallback(C.callProgressCallback))

	if int(C.cdecrypt_main(2, (**C.char)(unsafe.Pointer(&argv[0])))) != 0 {
		return fmt.Errorf("decryption failed")
	}

	if deleteEncryptedContents {
		doDeleteEncryptedContents(path)
	}

	return nil
}
