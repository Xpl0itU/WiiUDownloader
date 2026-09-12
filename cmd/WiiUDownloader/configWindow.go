package main

import (
	"context"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ConfigWindow struct {
	Window *gtk.Window
	Config *Config
}

const (
	SETTINGS_WINDOW_WIDTH               = 540
	SETTINGS_WINDOW_HEIGHT              = 620
	UNSAVED_CHANGES_CONFIRM_MESSAGE     = "You have unsaved changes. Close without saving?"
	INVALID_DOWNLOAD_PATH_ERROR_MESSAGE = "Invalid download path. Please select a valid directory."
)

// NewConfigWindow builds the preferences window out of libadwaita preference
// pages, so every setting is a native-looking row instead of a hand-laid grid.
func NewConfigWindow(config *Config) (*ConfigWindow, error) {
	win := adw.NewWindow()
	win.SetTitle(WINDOW_TITLE_PREFIX + "Settings")
	win.SetDefaultSize(SETTINGS_WINDOW_WIDTH, SETTINGS_WINDOW_HEIGHT)
	win.AddCSSClass("settings-window")
	// The GTK-facing helpers take a *gtk.Window; this is the same object.
	parentWindow := &win.Window

	header := adw.NewHeaderBar()

	// --- Storage ---
	storage := adw.NewPreferencesGroup()
	storage.SetTitle("Storage")

	downloadPathRow := adw.NewEntryRow()
	downloadPathRow.SetTitle("Download path")
	downloadPathRow.SetText(config.LastSelectedPath)
	setTooltip(downloadPathRow, composeAccessibleText("Download path", "Location where downloaded games will be saved.", ". "))
	downloadBrowseButton := gtk.NewButtonWithLabel("Browse")
	downloadBrowseButton.SetVAlign(gtk.AlignCenter)
	SetupButtonAccessibility(downloadBrowseButton, "Browse for download path")
	downloadBrowseButton.ConnectClicked(func() {
		chooseFolder(parentWindow, WINDOW_TITLE_PREFIX+"Select Download Path", "", func(selectedPath string) {
			if selectedPath != "" {
				downloadPathRow.SetText(selectedPath)
			}
		})
	})
	downloadPathRow.AddSuffix(downloadBrowseButton)
	storage.Add(downloadPathRow)

	rememberPathRow := adw.NewSwitchRow()
	rememberPathRow.SetTitle("Remember last used location")
	rememberPathRow.SetActive(config.RememberLastPath)
	setTooltip(rememberPathRow, composeAccessibleText("Remember last used location", "Automatically save files to the last used location.", ". "))
	storage.Add(rememberPathRow)

	decryptPathRow := adw.NewEntryRow()
	decryptPathRow.SetTitle("Decrypted output path")
	decryptPathRow.SetText(config.DecryptOutputPath)
	setTooltip(decryptPathRow, composeAccessibleText("Decrypted output path", "Optional folder where decrypted game files will be saved. Leave empty to save alongside downloads.", ". "))
	decryptBrowseButton := gtk.NewButtonWithLabel("Browse")
	decryptBrowseButton.SetVAlign(gtk.AlignCenter)
	SetupButtonAccessibility(decryptBrowseButton, "Browse for decrypted output path")
	decryptBrowseButton.ConnectClicked(func() {
		chooseFolder(parentWindow, WINDOW_TITLE_PREFIX+"Select Decrypted Output Path", "", func(selectedPath string) {
			if selectedPath != "" {
				decryptPathRow.SetText(selectedPath)
			}
		})
	})
	decryptPathRow.AddSuffix(decryptBrowseButton)

	clearDecryptButton := gtk.NewButtonWithLabel("Clear")
	clearDecryptButton.SetVAlign(gtk.AlignCenter)
	clearDecryptButton.AddCSSClass("destructive-action")
	SetupButtonAccessibility(clearDecryptButton, "Clear the decrypted files output path")
	clearDecryptButton.ConnectClicked(func() {
		decryptPathRow.SetText("")
	})
	decryptPathRow.AddSuffix(clearDecryptButton)
	storage.Add(decryptPathRow)

	// --- Downloads ---
	downloads := adw.NewPreferencesGroup()
	downloads.SetTitle("Downloads")

	continueOnErrorRow := adw.NewSwitchRow()
	continueOnErrorRow.SetTitle("Continue downloading on errors")
	continueOnErrorRow.SetSubtitle("Show a summary of failed titles at the end")
	continueOnErrorRow.SetActive(config.ContinueOnError)
	setTooltip(continueOnErrorRow, composeAccessibleText("Continue downloading on errors", "Continue with remaining titles even if some fail.", ". "))
	downloads.Add(continueOnErrorRow)

	inlineProgressRow := adw.NewSwitchRow()
	inlineProgressRow.SetTitle("Show download progress in the queue pane")
	inlineProgressRow.SetSubtitle("Off: downloads report in a separate progress window")
	inlineProgressRow.SetActive(config.UseInlineDownloadUI)
	setTooltip(inlineProgressRow, composeAccessibleText("Show download progress in the queue pane", "Report the running download inside the queue pane instead of a separate progress window.", ". "))
	downloads.Add(inlineProgressRow)

	suggestRelatedRow := adw.NewSwitchRow()
	suggestRelatedRow.SetTitle("Suggest related content")
	suggestRelatedRow.SetSubtitle("Offer matching Game, DLC and Update entries when queueing")
	suggestRelatedRow.SetActive(config.SuggestRelatedContent)
	setTooltip(suggestRelatedRow, composeAccessibleText("Suggest related content", "Offer related content that matches the same title ID.", ". "))
	downloads.Add(suggestRelatedRow)

	// --- Interface ---
	interfaceGroup := adw.NewPreferencesGroup()
	interfaceGroup.SetTitle("Interface")

	darkModeRow := adw.NewSwitchRow()
	darkModeRow.SetTitle("Dark mode")
	darkModeRow.SetActive(config.DarkMode)
	setTooltip(darkModeRow, composeAccessibleText("Dark mode", "Enable dark theme for the interface.", ". "))
	interfaceGroup.Add(darkModeRow)

	showDonationBarRow := adw.NewSwitchRow()
	showDonationBarRow.SetTitle("Show donation banner")
	showDonationBarRow.SetActive(config.ShowDonationBar)
	setTooltip(showDonationBarRow, composeAccessibleText("Show donation banner", "Show a small banner at the bottom to support the project.", ". "))
	interfaceGroup.Add(showDonationBarRow)

	getSizeOnQueueRow := adw.NewSwitchRow()
	getSizeOnQueueRow.SetTitle("Fetch game size when adding to queue")
	getSizeOnQueueRow.SetActive(config.GetSizeOnQueue)
	setTooltip(getSizeOnQueueRow, composeAccessibleText("Fetch game size when adding to queue", "Automatically calculate game size using the TMD file when added to queue.", ". "))
	interfaceGroup.Add(getSizeOnQueueRow)

	page := adw.NewPreferencesPage()
	page.Add(storage)
	page.Add(downloads)
	page.Add(interfaceGroup)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(page)
	scrolled.SetVExpand(true)

	// --- Actions ---
	closeButton := gtk.NewButtonWithLabel("Close")
	SetupButtonAccessibility(closeButton, "Close settings window without saving changes")

	saveButton := gtk.NewButtonWithLabel("Save and Apply")
	saveButton.AddCSSClass("suggested-action")
	saveButton.SetReceivesDefault(true)
	SetupButtonAccessibility(saveButton, "Save all configuration changes and apply them immediately")

	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(6)
	buttonBox.SetMarginBottom(6)
	buttonBox.SetMarginStart(12)
	buttonBox.SetMarginEnd(12)
	buttonBox.Append(closeButton)
	buttonBox.Append(saveButton)

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(scrolled)
	toolbar.AddBottomBar(buttonBox)
	win.SetContent(toolbar)
	win.SetDefaultWidget(saveButton)

	// Comparing the rows against the config is the dirty check; no per-widget
	// change bookkeeping is needed, and it cannot drift out of sync.
	windowIsDirty := func() bool {
		return downloadPathRow.Text() != config.LastSelectedPath ||
			decryptPathRow.Text() != config.DecryptOutputPath ||
			rememberPathRow.Active() != config.RememberLastPath ||
			continueOnErrorRow.Active() != config.ContinueOnError ||
			inlineProgressRow.Active() != config.UseInlineDownloadUI ||
			suggestRelatedRow.Active() != config.SuggestRelatedContent ||
			showDonationBarRow.Active() != config.ShowDonationBar ||
			getSizeOnQueueRow.Active() != config.GetSizeOnQueue ||
			darkModeRow.Active() != config.DarkMode
	}

	saveButton.ConnectClicked(func() {
		config.DarkMode = darkModeRow.Active()
		newPath := downloadPathRow.Text()
		if newPath != "" && !isValidPath(newPath) {
			showAlert(parentWindow, WINDOW_TITLE_PREFIX+"Error", INVALID_DOWNLOAD_PATH_ERROR_MESSAGE)
			return
		}

		config.LastSelectedPath = newPath
		config.RememberLastPath = rememberPathRow.Active()
		config.ContinueOnError = continueOnErrorRow.Active()
		config.UseInlineDownloadUI = inlineProgressRow.Active()
		config.SuggestRelatedContent = suggestRelatedRow.Active()
		config.ShowDonationBar = showDonationBarRow.Active()
		config.GetSizeOnQueue = getSizeOnQueueRow.Active()
		config.DecryptOutputPath = decryptPathRow.Text()

		setButtonsSensitive(false, saveButton, closeButton)

		go func() {
			err := config.Save()

			uiIdleAdd(func() {
				setButtonsSensitive(true, saveButton, closeButton)

				if err != nil {
					ShowErrorDialog(parentWindow, err)
				}
			})
		}()
	})

	closeWindow := func() {
		win.SetVisible(false)
	}
	closeButton.ConnectClicked(func() {
		if !windowIsDirty() {
			closeWindow()
			return
		}
		confirmCloseWithoutSaving(parentWindow, closeWindow)
	})
	// Returning true blocks the default close for the async confirmation.
	win.ConnectCloseRequest(func() bool {
		if !windowIsDirty() {
			return false
		}
		confirmCloseWithoutSaving(parentWindow, closeWindow)
		return true
	})

	return &ConfigWindow{Window: parentWindow, Config: config}, nil
}

func setButtonsSensitive(sensitive bool, buttons ...*gtk.Button) {
	for _, button := range buttons {
		if button != nil {
			button.SetSensitive(sensitive)
		}
	}
}

func confirmCloseWithoutSaving(parent *gtk.Window, onDiscard func()) {
	dialog := adw.NewAlertDialog(WINDOW_TITLE_PREFIX+"Unsaved Changes", UNSAVED_CHANGES_CONFIRM_MESSAGE)
	dialog.AddResponse("cancel", "Keep Editing")
	dialog.AddResponse("discard", "Discard")
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")
	dialog.SetResponseAppearance("discard", adw.ResponseDestructive)

	var anchor gtk.Widgetter
	if parent != nil {
		anchor = parent
	}
	dialog.Choose(context.Background(), anchor, func(res gio.AsyncResulter) {
		if dialog.ChooseFinish(res) == "discard" && onDiscard != nil {
			onDiscard()
		}
	})
}
