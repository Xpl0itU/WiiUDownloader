// Copyright (C) 2019 Vincent Chueng (coolingfall@gmail.com).

package aria2go

/*
 #cgo CXXFLAGS: -std=c++11 -I./aria2-lib/include -Werror -Wall
 #cgo LDFLAGS: -L./aria2-lib/lib
 #cgo LDFLAGS: -lcrypto -lssl -lcares -lz -laria2 -framework Security
 #include <stdlib.h>
 #include "aria2_c.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	_ "github.com/benesch/cgosymbolizer"
)

// Type definition for lib aria2, it holds a notifier.
type Aria2 struct {
	notifier             Notifier
	shutdownNotification chan bool
	shouldShutdown       bool
	m_mutex              sync.Mutex
}

// Type definition of configuration for aria2.
type Config struct {
	Options  Options
	Notifier Notifier
}

// NewAria2 creates a new instance of aria2.
func NewAria2(config Config) *Aria2 {
	a := &Aria2{
		notifier:             newDefaultNotifier(),
		shutdownNotification: make(chan bool),
	}
	a.SetNotifier(config.Notifier)

	C.init(C.uint64_t(uintptr(unsafe.Pointer(a))),
		C.CString(a.fromOptions(config.Options)))
	return a
}

// Shutdown aria2, this must be invoked when process exit(signal handler is not
// used), so aria2 will be able to save session config.
func (a *Aria2) Shutdown() int {
	C.shutdownSchedules(true)
	a.shouldShutdown = true

	// do nothing, just make thread waiting
	select {
	case <-a.shutdownNotification:
		break
	}

	return int(C.deinit())
}

// Run starts event pooling. Note this will block current thread.
func (a *Aria2) Run() {
	for {
		if C.run() != 1 && a.shouldShutdown {
			break
		}
	}
	a.shutdownNotification <- true
}

// SetNotifier sets notifier to receive download notification from aria2.
func (a *Aria2) SetNotifier(notifier Notifier) {
	if notifier == nil {
		return
	}
	a.notifier = notifier
}

// AddUri adds a new download. The uris is an array of HTTP/FTP/SFTP/BitTorrent
// URIs (strings) pointing to the same resource. When adding BitTorrent Magnet
// URIs, uris must have only one element and it should be BitTorrent Magnet URI.
func (a *Aria2) AddUri(uri string, options Options) (gid string, err error) {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	cUri := C.CString(uri)
	cOptions := C.CString(a.fromOptions(options))
	defer C.free(unsafe.Pointer(cUri))
	defer C.free(unsafe.Pointer(cOptions))

	ret := C.addUri(cUri, cOptions)
	if ret == 0 {
		return "", errors.New("libaria2: add uri failed")
	}
	return fmt.Sprintf("%x", uint64(ret)), nil
}

// AddTorrent adds a MetaInfo download with given torrent file path.
// This will return gid and files in torrent file if add successfully.
// User can choose specified files to download, change directory and so on.
func (a *Aria2) AddTorrent(filepath string, options Options) (gid string, err error) {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	cFilepath := C.CString(filepath)
	cOptions := C.CString(a.fromOptions(options))
	defer C.free(unsafe.Pointer(cFilepath))
	defer C.free(unsafe.Pointer(cOptions))

	ret := C.addTorrent(cFilepath, cOptions)
	if ret == 0 {
		return "", errors.New("libaria2: add torrent failed")
	}
	return fmt.Sprintf("%x", uint64(ret)), nil
}

// ChangeOptions can change the options for aria2. See available options in
// https://aria2.github.io/manual/en/html/aria2c.html#input-file.
func (a *Aria2) ChangeOptions(gid string, options Options) error {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	cOptions := C.CString(a.fromOptions(options))
	defer C.free(unsafe.Pointer(cOptions))

	if !C.changeOptions(a.hexToGid(gid), cOptions) {
		return errors.New("libaria2: change options error")
	}

	return nil
}

// GetOptions gets all options for given gid.
func (a *Aria2) GetOptions(gid string) Options {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	cOptions := C.getOptions(a.hexToGid(gid))
	if cOptions == nil {
		return make(Options)
	}

	return a.toOptions(C.GoString(cOptions))
}

// ChangeGlobalOptions changes global options. See available options in
// https://aria2.github.io/manual/en/html/aria2c.html#input-file except for
// `checksum`, `index-out`, `out`, `pause` and `select-file`.
func (a *Aria2) ChangeGlobalOptions(options Options) error {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	cOptions := C.CString(a.fromOptions(options))
	defer C.free(unsafe.Pointer(cOptions))

	if !C.changeGlobalOptions(cOptions) {
		return errors.New("libaria2: change global options error")
	}

	return nil
}

// GetGlobalOptions gets all global options of aria2.
func (a *Aria2) GetGlobalOptions() Options {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	return a.toOptions(C.GoString(C.getGlobalOptions()))
}

// Pause pauses an active download for given gid. The status of the download
// will become `DOWNLOAD_PAUSED`. Use `Resume` to restart download.
func (a *Aria2) Pause(gid string) bool {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	return bool(C.pause(a.hexToGid(gid)))
}

// Resume resumes an paused download for given gid.
func (a *Aria2) Resume(gid string) bool {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	return bool(C.resume(a.hexToGid(gid)))
}

// Remove removes download no matter what status it was. This will stop
// downloading and stop seeding(for torrent).
func (a *Aria2) Remove(gid string) bool {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	return bool(C.removeDownload(a.hexToGid(gid)))
}

// GetDownloadInfo gets current download information for given gid.
func (a *Aria2) GetDownloadInfo(gid string) DownloadInfo {
	a.m_mutex.Lock()
	defer a.m_mutex.Unlock()
	ret := C.getDownloadInfo(a.hexToGid(gid))
	if ret == nil {
		return DownloadInfo{}
	}
	defer C.free(unsafe.Pointer(ret))

	// convert info hash to hex string
	infoHash := fmt.Sprintf("%x", []byte(C.GoString(ret.infoHash)))
	C.free(unsafe.Pointer(ret.infoHash))
	// retrieve BitTorrent meta information
	var metaInfo = MetaInfo{}
	mi := ret.metaInfo
	defer C.free(unsafe.Pointer(mi))
	if mi != nil {
		announceList := strings.Split(C.GoString(mi.announceList), ";")
		metaInfo = MetaInfo{
			Name:         C.GoString(mi.name),
			Comment:      C.GoString(mi.comment),
			CreationUnix: int64(mi.creationUnix),
			AnnounceList: announceList,
		}
		C.free(unsafe.Pointer(mi.name))
		C.free(unsafe.Pointer(mi.comment))
		C.free(unsafe.Pointer(mi.announceList))
	}
	return DownloadInfo{
		Status:         int(ret.status),
		TotalLength:    int64(ret.totalLength),
		BytesCompleted: int64(ret.bytesCompleted),
		BytesUpload:    int64(ret.uploadLength),
		DownloadSpeed:  int(ret.downloadSpeed),
		UploadSpeed:    int(ret.uploadSpeed),
		NumPieces:      int(ret.numPieces),
		Connections:    int(ret.connections),
		InfoHash:       infoHash,
		MetaInfo:       metaInfo,
		Files:          a.parseFiles(ret.files, ret.numFiles),
		ErrorCode:      int(ret.errorCode),
		FollowedByGid:  fmt.Sprintf("%x", uint64(ret.followedByGid)),
	}
}

// fromOptions converts `Options` to string with ';' separator.
func (a *Aria2) fromOptions(options Options) string {
	if options == nil {
		return ""
	}

	var cOptions string
	for k, v := range options {
		cOptions += k + ";"
		cOptions += v + ";"
	}

	return strings.TrimSuffix(cOptions, ";")
}

// fromOptions converts options string with ';' separator to `Options`.
func (a *Aria2) toOptions(cOptions string) Options {
	coptions := strings.Split(strings.TrimSuffix(cOptions, ";"), ";")
	var options = make(Options)
	var index int
	for index = 0; index < len(coptions); index += 2 {
		options[coptions[index]] = coptions[index+1]
	}

	return options
}

// hexToGid convert hex to uint64 type gid.
func (a *Aria2) hexToGid(hex string) C.uint64_t {
	id, err := strconv.ParseUint(hex, 16, 64)
	if err != nil {
		return 0
	}
	return C.uint64_t(id)
}

// parseFiles parses all files information from aria2.
func (a *Aria2) parseFiles(filesPointer *C.struct_FileInfo, length C.int) (files []File) {
	cfiles := (*[1 << 20]C.struct_FileInfo)(unsafe.Pointer(filesPointer))[:length:length]
	if cfiles == nil {
		return
	}

	for _, f := range cfiles {
		files = append(files, File{
			Index:           int(f.index),
			Length:          int64(f.length),
			CompletedLength: int64(f.completedLength),
			Name:            C.GoString(f.name),
			Selected:        bool(f.selected),
		})
		C.free(unsafe.Pointer(f.name))
	}

	// free c pointer resource
	C.free(unsafe.Pointer(filesPointer))

	return
}

// noinspection GoUnusedFunction
//
//export notifyEvent
func notifyEvent(ariagoPointer uint64, id uint64, event int) {
	a := (*Aria2)(unsafe.Pointer(uintptr(ariagoPointer)))
	if a == nil || a.notifier == nil {
		return
	}

	// convert id to hex string
	gid := fmt.Sprintf("%x", uint64(id))

	switch event {
	case onStart:
		a.notifier.OnStart(gid)
	case onPause:
		a.notifier.OnPause(gid)
	case onStop:
		a.notifier.OnStop(gid)
	case onComplete:
		a.notifier.OnComplete(gid)
	case onError:
		a.notifier.OnError(gid)
	}
}

// noinspection GoUnusedFunction
//
//export goLog
func goLog(msg *C.char) {
	log.Println(C.GoString(msg))
}
