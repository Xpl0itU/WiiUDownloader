package main

import (
	"github.com/Xpl0itU/dialog"
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
	win.SetTitle("WiiUDownloader - Settings")
	win.SetDecorated(true)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetDefaultSize(420, 200)

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetVAlign(gtk.ALIGN_CENTER)
	grid.SetHAlign(gtk.ALIGN_CENTER)
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(8)
	grid.SetMarginTop(12)
	grid.SetMarginBottom(12)
	grid.SetMarginStart(12)
	if sc, _ := win.GetStyleContext(); sc != nil {
		sc.AddClass("settings-window")
	}
	if gsc, _ := grid.GetStyleContext(); gsc != nil {
		gsc.AddClass("settings-grid")
	}
	grid.SetMarginEnd(12)
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
		selectedPath, err := dialog.Directory().Title("Select Download Path").Browse()
		if err != nil {
			return
		}
		if selectedPath != "" {
			downloadPathEntry.SetText(selectedPath)
		}
	})

	rememberPathCheck, err := gtk.CheckButtonNewWithLabel("Automatically save files to last used location")
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
	saveButton.SetHAlign(gtk.ALIGN_END)
	grid.AttachNextTo(saveButton, rememberPathCheck, gtk.POS_BOTTOM, 1, 1)
	closeButton, err := gtk.ButtonNewWithLabel("Close")
	if err != nil {
		return nil, err
	}
	closeButton.SetMarginTop(10)
	closeButton.SetHAlign(gtk.ALIGN_END)
	grid.AttachNextTo(closeButton, saveButton, gtk.POS_RIGHT, 1, 1)

	dirty := false
	darkModeCheck.Connect("toggled", func() { dirty = true })
	rememberPathCheck.Connect("toggled", func() { dirty = true })
	downloadPathEntry.Connect("changed", func() { dirty = true })

	saveButton.Connect("clicked", func() {
		config.DarkMode = darkModeCheck.GetActive()
		newPath, _ := downloadPathEntry.GetText()
		if newPath != "" && !isValidPath(newPath) {
			errorDialog := gtk.MessageDialogNew(win, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "Invalid download path. Please select a valid directory.")
			defer errorDialog.Destroy()
			errorDialog.Run()
			return
		}

		config.LastSelectedPath = newPath
		config.RememberLastPath = rememberPathCheck.GetActive()

		if err := config.Save(); err != nil {
			ShowErrorDialog(win, err)
		}
		dirty = false
	})
	closeButton.Connect("clicked", func() {
		if dirty {
			confirm := gtk.MessageDialogNew(win, gtk.DIALOG_MODAL, gtk.MESSAGE_WARNING, gtk.BUTTONS_YES_NO, "You have unsaved changes. Close without saving?")
			resp := confirm.Run()
			confirm.Destroy()
			if resp != gtk.RESPONSE_YES {
				return
			}
		}
		win.Hide()
	})
	win.Connect("delete-event", func() bool {
		if dirty {
			confirm := gtk.MessageDialogNew(win, gtk.DIALOG_MODAL, gtk.MESSAGE_WARNING, gtk.BUTTONS_YES_NO, "You have unsaved changes. Close without saving?")
			resp := confirm.Run()
			confirm.Destroy()
			return resp != gtk.RESPONSE_YES
		}
		return false
	})

	win.SetBorderWidth(10)

	configWindow := ConfigWindow{
		Window: win,
		Config: config,
	}

	return &configWindow, nil
}
