package main

import (
	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/gtk"
)

type ConfigWindow struct {
	Window *gtk.Window
	Config *Config
}

const (
	SETTINGS_WINDOW_WIDTH               = 480
	SETTINGS_WINDOW_HEIGHT              = 320
	SETTINGS_GRID_MIN_WIDTH             = 440
	SETTINGS_GRID_MARGIN                = 12
	SETTINGS_FIELD_MARGIN_TOP           = 10
	SETTINGS_ENTRY_WIDTH_CHARS          = 35
	SETTINGS_ENTRY_MARGIN_END           = 10
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
	addStyleClass(win.GetStyleContext, "settings-window")

	mainBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)
	if err != nil {
		return nil, err
	}
	mainBox.SetMarginTop(SETTINGS_GRID_MARGIN)
	mainBox.SetMarginBottom(SETTINGS_GRID_MARGIN)
	mainBox.SetMarginStart(SETTINGS_GRID_MARGIN)
	mainBox.SetMarginEnd(SETTINGS_GRID_MARGIN)
	win.Add(mainBox)

	notebook, err := gtk.NotebookNew()
	if err != nil {
		return nil, err
	}
	mainBox.PackStart(notebook, true, true, 0)

	// --- General Tab ---
	generalGrid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	generalGrid.SetRowSpacing(12)
	generalGrid.SetColumnSpacing(12)
	generalGrid.SetMarginTop(12)
	generalGrid.SetMarginBottom(12)
	generalGrid.SetMarginStart(12)
	generalGrid.SetMarginEnd(12)

	downloadPathLabel, err := gtk.LabelNew("Download Path:")
	if err != nil {
		return nil, err
	}
	downloadPathLabel.SetHAlign(gtk.ALIGN_START)
	generalGrid.Attach(downloadPathLabel, 0, 0, 2, 1)

	downloadPathEntry, err := gtk.EntryNew()
	if err != nil {
		return nil, err
	}
	downloadPathEntry.SetText(config.LastSelectedPath)
	downloadPathEntry.SetWidthChars(SETTINGS_ENTRY_WIDTH_CHARS)
	downloadPathEntry.SetHExpand(true)
	SetupEntryAccessibility(downloadPathEntry, "Download path", "Location where downloaded games will be saved.")
	generalGrid.Attach(downloadPathEntry, 0, 1, 1, 1)

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
	generalGrid.Attach(downloadPathButton, 1, 1, 1, 1)

	rememberPathCheck, err := gtk.CheckButtonNewWithLabel("Automatically save files to last used location")
	if err != nil {
		return nil, err
	}
	rememberPathCheck.SetActive(config.RememberLastPath)
	SetupCheckButtonAccessibility(rememberPathCheck, "Remember and automatically use the last download location")
	generalGrid.Attach(rememberPathCheck, 0, 2, 2, 1)

	generalTabLabel, _ := gtk.LabelNew("General")
	notebook.AppendPage(generalGrid, generalTabLabel)

	// --- Downloads Tab ---
	downloadsGrid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	downloadsGrid.SetRowSpacing(12)
	downloadsGrid.SetMarginTop(12)
	downloadsGrid.SetMarginBottom(12)
	downloadsGrid.SetMarginStart(12)
	downloadsGrid.SetMarginEnd(12)

	continueOnErrorCheck, err := gtk.CheckButtonNewWithLabel("Continue downloading on errors (show summary at end)")
	if err != nil {
		return nil, err
	}
	continueOnErrorCheck.SetActive(config.ContinueOnError)
	SetupCheckButtonAccessibility(continueOnErrorCheck, "Continue with remaining titles even if some fail")
	downloadsGrid.Attach(continueOnErrorCheck, 0, 0, 1, 1)

	suggestRelatedContentCheck, err := gtk.CheckButtonNewWithLabel("Suggest related Game/DLC/Update when queueing")
	if err != nil {
		return nil, err
	}
	suggestRelatedContentCheck.SetActive(config.SuggestRelatedContent)
	SetupCheckButtonAccessibility(suggestRelatedContentCheck, "Offer related content that matches the same title ID")
	downloadsGrid.Attach(suggestRelatedContentCheck, 0, 1, 1, 1)

	downloadsTabLabel, _ := gtk.LabelNew("Downloads")
	notebook.AppendPage(downloadsGrid, downloadsTabLabel)

	// --- Interface Tab ---
	interfaceGrid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	interfaceGrid.SetRowSpacing(12)
	interfaceGrid.SetMarginTop(12)
	interfaceGrid.SetMarginBottom(12)
	interfaceGrid.SetMarginStart(12)
	interfaceGrid.SetMarginEnd(12)

	darkModeCheck, err := gtk.CheckButtonNewWithLabel("Dark Mode")
	if err != nil {
		return nil, err
	}
	darkModeCheck.SetActive(config.DarkMode)
	SetupCheckButtonAccessibility(darkModeCheck, "Enable dark theme for the interface")
	interfaceGrid.Attach(darkModeCheck, 0, 0, 1, 1)

	showDonationBarCheck, err := gtk.CheckButtonNewWithLabel("Show support nudge")
	if err != nil {
		return nil, err
	}
	showDonationBarCheck.SetActive(config.ShowDonationBar)
	SetupCheckButtonAccessibility(showDonationBarCheck, "Show a small bar at the bottom to support the project")
	interfaceGrid.Attach(showDonationBarCheck, 0, 1, 1, 1)

	getSizeOnQueueCheck, err := gtk.CheckButtonNewWithLabel("Fetch game size when adding to queue")
	if err != nil {
		return nil, err
	}
	getSizeOnQueueCheck.SetActive(config.GetSizeOnQueue)
	SetupCheckButtonAccessibility(getSizeOnQueueCheck, "Automatically calculate game size using TMD file when added to queue")
	interfaceGrid.Attach(getSizeOnQueueCheck, 0, 2, 1, 1)

	interfaceTabLabel, _ := gtk.LabelNew("Interface")
	notebook.AppendPage(interfaceGrid, interfaceTabLabel)

	// --- Action Buttons ---
	buttonBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		return nil, err
	}
	buttonBox.SetHAlign(gtk.ALIGN_END)
	mainBox.PackEnd(buttonBox, false, false, 0)

	saveButton, err := gtk.ButtonNewWithLabel("Save and Apply")
	if err != nil {
		return nil, err
	}
	saveButton.SetCanDefault(true)
	SetupButtonAccessibility(saveButton, "Save all configuration changes and apply them immediately")
	buttonBox.PackStart(saveButton, false, false, 0)

	closeButton, err := gtk.ButtonNewWithLabel("Close")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(closeButton, "Close settings window without saving changes")
	buttonBox.PackStart(closeButton, false, false, 0)

	dirty := false
	darkModeCheck.Connect("toggled", func() { dirty = true })
	rememberPathCheck.Connect("toggled", func() { dirty = true })
	continueOnErrorCheck.Connect("toggled", func() { dirty = true })
	suggestRelatedContentCheck.Connect("toggled", func() { dirty = true })
	showDonationBarCheck.Connect("toggled", func() { dirty = true })
	getSizeOnQueueCheck.Connect("toggled", func() { dirty = true })
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
		config.ShowDonationBar = showDonationBarCheck.GetActive()
		config.GetSizeOnQueue = getSizeOnQueueCheck.GetActive()

		setButtonsSensitive(false, saveButton, closeButton)

		go func() {
			err := config.Save()

			uiIdleAdd(func() {
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
