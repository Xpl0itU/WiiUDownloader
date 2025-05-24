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

	downloadPathLabel, err := gtk.LabelNew("Download Path:")
	if err != nil {
		return nil, err
	}
	downloadPathLabel.SetHAlign(gtk.ALIGN_START)
	downloadPathLabel.SetMarginTop(10)
	grid.AttachNextTo(downloadPathLabel, darkModeCheck, gtk.POS_BOTTOM, 1, 1)

	downloadPathEntry, err := gtk.EntryNew()
	if err != nil {
		return nil, err
	}
	downloadPathEntry.SetText(config.LastSelectedPath)
	downloadPathEntry.SetWidthChars(40)
	downloadPathEntry.SetMarginEnd(10)
	grid.AttachNextTo(downloadPathEntry, downloadPathLabel, gtk.POS_BOTTOM, 1, 1)

	downloadPathButton, err := gtk.ButtonNewWithLabel("Browse")
	if err != nil {
		return nil, err
	}
	downloadPathButton.Connect("clicked", func() {
		dialog, err := gtk.FileChooserDialogNewWith2Buttons("Select Download Path", win, gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER, "Select", gtk.RESPONSE_ACCEPT, "Cancel", gtk.RESPONSE_CANCEL)
		if err != nil {
			log.Println(err)
			return
		}
		defer dialog.Destroy()
		if dialog.Run() == gtk.RESPONSE_ACCEPT {
			downloadPathEntry.SetText(dialog.GetFilename())
		}
	})
	grid.AttachNextTo(downloadPathButton, downloadPathEntry, gtk.POS_RIGHT, 1, 1)

	rememberPathCheck, err := gtk.CheckButtonNewWithLabel("Remember last path, do not ask every time")
	if err != nil {
		return nil, err
	}
	rememberPathCheck.SetActive(config.RememberLastPath)
	rememberPathCheck.SetMarginTop(10)
	grid.AttachNextTo(rememberPathCheck, downloadPathEntry, gtk.POS_BOTTOM, 1, 1)

	saveButton, err := gtk.ButtonNewWithLabel("Save and Apply")
	if err != nil {
		return nil, err
	}
	saveButton.SetMarginTop(10)
	grid.AttachNextTo(saveButton, rememberPathCheck, gtk.POS_BOTTOM, 1, 1)

	saveButton.Connect("clicked", func() {
		config.DarkMode = darkModeCheck.GetActive()

		var newPath = downloadPathEntry.GetLayout().GetText()
		if newPath != "" && !isValidPath(newPath) {
			errorDialog := gtk.MessageDialogNew(win, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "Invalid download path. Please select a valid directory.")
			defer errorDialog.Destroy()
			errorDialog.Run()
			return
		}

		config.LastSelectedPath = newPath
		config.RememberLastPath = rememberPathCheck.GetActive()

		if err := config.Save(); err != nil {
			log.Println(err)
		}
	})

	win.SetDefaultSize(grid.GetAllocatedWidth()+125, grid.GetAllocatedHeight()+70)
	win.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	win.SetBorderWidth(15)

	configWindow := ConfigWindow{
		Window: win,
		Config: config,
	}

	return &configWindow, nil
}
