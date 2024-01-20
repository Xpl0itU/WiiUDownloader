// Copyright (C) 2019 Vincent Chueng (coolingfall@gmail.com).

package aria2go

import (
	"log"
)

// Type definition for notifier which can be used in aria2c json rpc notification.
type Notifier interface {
	// OnStart will be invoked when aria2 started to download
	OnStart(gid string)

	// OnPause will be invoked when aria2 paused one download
	OnPause(gid string)

	// OnPause will be invoked when aria2 stopped one download
	OnStop(gid string)

	// OnComplete will be invoked when download completed
	OnComplete(gid string)

	// OnError will be invoked when an error occoured
	OnError(gid string)
}

// Type definition for default notifier which dose nothing.
type DefaultNotifier struct{}

// newDefaultNotifier creates a new instance of default Notifier.
func newDefaultNotifier() Notifier {
	return DefaultNotifier{}
}

// OnStart implements Notifier interface.
func (n DefaultNotifier) OnStart(gid string) {
	log.Printf("on start %v", gid)
}

// OnPause implements Notifier interface.
func (n DefaultNotifier) OnPause(gid string) {
	log.Printf("on pause: %v", gid)
}

// OnPause implements Notifier interface.
func (n DefaultNotifier) OnStop(gid string) {
	log.Printf("on stop: %v", gid)
}

// OnComplete implements Notifier interface.
func (n DefaultNotifier) OnComplete(gid string) {
	log.Printf("on complete: %v", gid)
}

// OnError implements Notifier interface.
func (n DefaultNotifier) OnError(gid string) {
	log.Printf("on error: %v", gid)
}
