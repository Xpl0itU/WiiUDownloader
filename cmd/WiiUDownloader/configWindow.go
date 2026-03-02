package main

import (
	"log"

	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type ConfigWindow struct {
	Window *gtk.Window
	Config *Config
}

const (
	SETTINGS_WINDOW_WIDTH               = 420
	SETTINGS_WINDOW_HEIGHT              = 200
	SETTINGS_GRID_MIN_WIDTH             = 450
	SETTINGS_GRID_MARGIN                = 12
	SETTINGS_FIELD_MARGIN_TOP           = 10
	SETTINGS_ENTRY_WIDTH_CHARS          = 40
	SETTINGS_ENTRY_MARGIN_END           = 10
	SETTINGS_DESCRIPTION_MAX_WIDTH      = 50
	UNSAVED_CHANGES_CONFIRM_MESSAGE     = "You have unsaved changes. Close without saving?"
	INVALID_DOWNLOAD_PATH_ERROR_MESSAGE = "Invalid download path. Please select a valid directory."
)

func NewConfigWindow(config *Config) (*ConfigWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("WiiUDownloader - Settings")
	win.SetDecorated(true)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetDefaultSize(SETTINGS_WINDOW_WIDTH, SETTINGS_WINDOW_HEIGHT)

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetVAlign(gtk.ALIGN_CENTER)
	grid.SetHAlign(gtk.ALIGN_CENTER)
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(8)
	grid.SetMarginTop(SETTINGS_GRID_MARGIN)
	grid.SetMarginBottom(SETTINGS_GRID_MARGIN)
	grid.SetMarginStart(SETTINGS_GRID_MARGIN)
	grid.SetMarginEnd(SETTINGS_GRID_MARGIN)
	grid.SetSizeRequest(SETTINGS_GRID_MIN_WIDTH, -1)
	addStyleClass(win.GetStyleContext, "settings-window")
	addStyleClass(grid.GetStyleContext, "settings-grid")
	win.Add(grid)
	darkModeCheck, err := gtk.CheckButtonNewWithLabel("Dark Mode")
	if err != nil {
		return nil, err
	}
	darkModeCheck.SetActive(config.DarkMode)
	SetupCheckButtonAccessibility(darkModeCheck, "Enable dark theme for the application interface")
	grid.Attach(darkModeCheck, 0, 0, 1, 1)
	downloadPathLabel, err := gtk.LabelNew("Download Path:")
	if err != nil {
		return nil, err
	}
	downloadPathLabel.SetHAlign(gtk.ALIGN_START)
	downloadPathLabel.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	grid.AttachNextTo(downloadPathLabel, darkModeCheck, gtk.POS_BOTTOM, 1, 1)
	downloadPathEntry, err := gtk.EntryNew()
	if err != nil {
		return nil, err
	}
	downloadPathEntry.SetText(config.LastSelectedPath)
	downloadPathEntry.SetWidthChars(SETTINGS_ENTRY_WIDTH_CHARS)
	downloadPathEntry.SetMarginEnd(SETTINGS_ENTRY_MARGIN_END)
	SetupEntryAccessibility(downloadPathEntry, "Download path", "Location where downloaded games will be saved. Click Browse to select a different directory")
	grid.AttachNextTo(downloadPathEntry, downloadPathLabel, gtk.POS_BOTTOM, 1, 1)
	downloadPathButton, err := gtk.ButtonNewWithLabel("Browse")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(downloadPathButton, "Open file browser to select download directory")
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
	rememberPathCheck.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	SetupCheckButtonAccessibility(rememberPathCheck, "When checked, the application will remember and automatically use the last download location")
	grid.AttachNextTo(rememberPathCheck, downloadPathEntry, gtk.POS_BOTTOM, 1, 1)

	continueOnErrorCheck, err := gtk.CheckButtonNewWithLabel("Continue downloading on errors (show summary at end)")
	if err != nil {
		return nil, err
	}
	continueOnErrorCheck.SetActive(config.ContinueOnError)
	continueOnErrorCheck.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	SetupCheckButtonAccessibility(continueOnErrorCheck, "When checked, the downloader will continue with remaining titles even if some fail, showing a summary of errors at the end")
	grid.AttachNextTo(continueOnErrorCheck, rememberPathCheck, gtk.POS_BOTTOM, 1, 1)

	suggestRelatedContentCheck, err := gtk.CheckButtonNewWithLabel("Suggest related Game/DLC/Update when queueing")
	if err != nil {
		return nil, err
	}
	suggestRelatedContentCheck.SetActive(config.SuggestRelatedContent)
	suggestRelatedContentCheck.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	SetupCheckButtonAccessibility(suggestRelatedContentCheck, "When checked, adding a game, update, or DLC to the queue will offer related content that matches the same title ID")
	grid.AttachNextTo(suggestRelatedContentCheck, continueOnErrorCheck, gtk.POS_BOTTOM, 1, 1)

	var lastWidget gtk.IWidget = suggestRelatedContentCheck

	saveButton, err := gtk.ButtonNewWithLabel("Save and Apply")
	if err != nil {
		return nil, err
	}
	saveButton.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	saveButton.SetHAlign(gtk.ALIGN_END)
	SetupButtonAccessibility(saveButton, "Save all configuration changes and apply them immediately")
	grid.AttachNextTo(saveButton, lastWidget, gtk.POS_BOTTOM, 1, 1)
	closeButton, err := gtk.ButtonNewWithLabel("Close")
	if err != nil {
		return nil, err
	}
	closeButton.SetMarginTop(SETTINGS_FIELD_MARGIN_TOP)
	closeButton.SetHAlign(gtk.ALIGN_END)
	SetupButtonAccessibility(closeButton, "Close settings window without saving changes")
	grid.AttachNextTo(closeButton, saveButton, gtk.POS_RIGHT, 1, 1)

	dirty := false
	darkModeCheck.Connect("toggled", func() { dirty = true })
	rememberPathCheck.Connect("toggled", func() { dirty = true })
	continueOnErrorCheck.Connect("toggled", func() { dirty = true })
	suggestRelatedContentCheck.Connect("toggled", func() { dirty = true })
	downloadPathEntry.Connect("changed", func() { dirty = true })

	saveButton.Connect("clicked", func() {
		config.DarkMode = darkModeCheck.GetActive()
		newPath, getTextErr := downloadPathEntry.GetText()
		if getTextErr != nil {
			ShowErrorDialog(win, getTextErr)
			return
		}
		if newPath != "" && !isValidPath(newPath) {
			errorDialog := gtk.MessageDialogNew(win, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, INVALID_DOWNLOAD_PATH_ERROR_MESSAGE)
			defer errorDialog.Destroy()
			errorDialog.Run()
			return
		}

		config.LastSelectedPath = newPath
		config.RememberLastPath = rememberPathCheck.GetActive()
		config.ContinueOnError = continueOnErrorCheck.GetActive()
		config.SuggestRelatedContent = suggestRelatedContentCheck.GetActive()

		setButtonsSensitive(false, saveButton, closeButton)

		go func() {
			err := config.Save()

			glib.IdleAdd(func() {
				setButtonsSensitive(true, saveButton, closeButton)

				if err != nil {
					ShowErrorDialog(win, err)
					return
				}

				dirty = false
			})
		}()
	})
	closeButton.Connect("clicked", func() {
		if dirty && !confirmCloseWithoutSaving(win) {
			return
		}
		win.Hide()
	})
	win.Connect("delete-event", func() bool {
		return dirty && !confirmCloseWithoutSaving(win)
	})

	win.SetBorderWidth(SETTINGS_FIELD_MARGIN_TOP)

	configWindow := ConfigWindow{
		Window: win,
		Config: config,
	}

	return &configWindow, nil
}

func addStyleClass(getStyleContext func() (*gtk.StyleContext, error), className string) {
	styleContext, err := getStyleContext()
	if err != nil || styleContext == nil {
		return
	}
	styleContext.AddClass(className)
}

func setButtonsSensitive(sensitive bool, buttons ...*gtk.Button) {
	for _, button := range buttons {
		if button != nil {
			button.SetSensitive(sensitive)
		}
	}
}

func confirmCloseWithoutSaving(parent *gtk.Window) bool {
	confirm := gtk.MessageDialogNew(parent, gtk.DIALOG_MODAL, gtk.MESSAGE_WARNING, gtk.BUTTONS_YES_NO, UNSAVED_CHANGES_CONFIRM_MESSAGE)
	response := confirm.Run()
	confirm.Destroy()
	return response == gtk.RESPONSE_YES
}
