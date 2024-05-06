package main

import (
	"log"

	"github.com/gotk3/gotk3/gtk"
)

type ConfigWindow struct {
	Window *gtk.Window
	Config *Config
}

func NewConfigWindow(config *Config) (*ConfigWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("WiiUDownloader - Config")

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetVAlign(gtk.ALIGN_CENTER)
	grid.SetHAlign(gtk.ALIGN_CENTER)
	win.Add(grid)

	darkModeCheck, err := gtk.CheckButtonNewWithLabel("Dark Mode")
	if err != nil {
		return nil, err
	}
	darkModeCheck.SetActive(config.DarkMode)
	grid.Attach(darkModeCheck, 0, 0, 1, 1)

	saveButton, err := gtk.ButtonNewWithLabel("Save and Apply")
	if err != nil {
		return nil, err
	}
	grid.AttachNextTo(saveButton, darkModeCheck, gtk.POS_BOTTOM, 1, 1)

	saveButton.Connect("clicked", func() {
		config.DarkMode = darkModeCheck.GetActive()
		if err := config.Save(); err != nil {
			log.Println(err)
		}
	})

	win.SetDefaultSize(grid.GetAllocatedWidth()+125, grid.GetAllocatedHeight()+70)

	configWindow := ConfigWindow{
		Window: win,
		Config: config,
	}

	return &configWindow, nil
}
