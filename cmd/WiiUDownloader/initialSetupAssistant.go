package main

import (
	"fmt"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type InitialSetupAssistantWindow struct {
	assistantWindow   *gtk.Assistant
	config            *Config
	skipButton        *gtk.Button
	nextButton        *gtk.Button
	backButton        *gtk.Button
	finishButton      *gtk.Button
	postSetupCallback func()
}

const (
	INITIAL_SETUP_WINDOW_WIDTH  = 600
	INITIAL_SETUP_WINDOW_HEIGHT = 500
	SETUP_PAGE_BORDER_WIDTH     = 24
	SETUP_PAGE_SPACING_LARGE    = 16
	SETUP_PAGE_SPACING          = 12
	SETUP_INFO_SPACING          = 8
	SETUP_ROW_HORIZONTAL_MARGIN = 16
	SETUP_ROW_VERTICAL_MARGIN   = 12
	SETUP_ROW_SPACING           = 12
	SETUP_SUB_TEXT_SPACING      = 2
	SETUP_SUMMARY_SPACING       = 4
	SETUP_SUMMARY_MARGIN        = 8
)

func NewInitialSetupAssistantWindow(config *Config) (*InitialSetupAssistantWindow, error) {
	var performPostSetup func()

	assistant, err := gtk.AssistantNew()
	if err != nil {
		return nil, err
	}
	assistant.Connect("cancel", func() {
		assistant.Destroy()
	})
	assistant.SetTitle("WiiUDownloader - Initial Setup")
	assistant.SetDefaultSize(INITIAL_SETUP_WINDOW_WIDTH, INITIAL_SETUP_WINDOW_HEIGHT)
	assistant.SetPosition(gtk.WIN_POS_CENTER)
	assistant.SetModal(true)

	actionBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		return nil, err
	}

	skipButton, err := gtk.ButtonNewWithLabel("Skip")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(skipButton, "Skip the initial setup wizard and start using the application with default settings")

	backButton, err := gtk.ButtonNewWithLabel("Back")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(backButton, "Go back to the previous step")

	nextButton, err := gtk.ButtonNewWithLabel("Next")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(nextButton, "Proceed to the next step")

	finishButton, err := gtk.ButtonNewWithLabel("Finish")
	if err != nil {
		return nil, err
	}
	SetupButtonAccessibility(finishButton, "Complete the initial setup")
	finishContext, err := finishButton.GetStyleContext()
	if err != nil {
		return nil, err
	}
	finishContext.AddClass("suggested-action")

	actionBox.PackStart(skipButton, false, false, 0)
	actionBox.PackStart(backButton, false, false, 0)
	actionBox.PackStart(nextButton, false, false, 0)
	actionBox.PackStart(finishButton, false, false, 0)

	assistant.AddActionWidget(actionBox)

	page1, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page1.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	page1.SetSpacing(SETUP_PAGE_SPACING_LARGE)

	page1Label, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page1Label.SetMarkup("<span font='18' weight='bold'>Welcome to WiiUDownloader</span>")
	page1Label.SetHAlign(gtk.ALIGN_START)
	page1.PackStart(page1Label, false, false, 0)

	page1SubLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page1SubLabel.SetMarkup("<span font='11' alpha='85%'>This setup wizard will guide you through the initial configuration in just a few steps. You can modify these settings anytime later in the preferences.</span>")
	page1SubLabel.SetLineWrap(true)
	page1SubLabel.SetHAlign(gtk.ALIGN_START)
	page1.PackStart(page1SubLabel, false, false, 0)

	spacer, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page1.PackStart(spacer, true, true, 0)

	infoBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	infoBox.SetSpacing(SETUP_INFO_SPACING)

	info1, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	info1.SetMarkup("<span font='10' alpha='80%'>▸ Select your preferred game regions</span>")
	info1.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(info1, false, false, 0)

	info2, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	info2.SetMarkup("<span font='10' alpha='80%'>▸ Choose target platforms (emulator and/or console)</span>")
	info2.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(info2, false, false, 0)

	info3, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	info3.SetMarkup("<span font='10' alpha='80%'>▸ Set the storage location for decrypted game files</span>")
	info3.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(info3, false, false, 0)

	info4, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	info4.SetMarkup("<span font='10' alpha='80%'>▸ Review and confirm your configuration</span>")
	info4.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(info4, false, false, 0)

	page1.PackStart(infoBox, false, false, 8)

	page2, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page2.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	page2.SetSpacing(SETUP_PAGE_SPACING)

	page2Label, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page2Label.SetMarkup("<span font='14' weight='bold'>Which regions do you want to download from?</span>")
	page2Label.SetHAlign(gtk.ALIGN_START)
	page2.PackStart(page2Label, false, false, 0)

	page2Desc, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page2Desc.SetMarkup("<span font='11' alpha='80%'>Select one or more regions to enable downloading games from their respective game libraries.</span>")
	page2Desc.SetHAlign(gtk.ALIGN_START)
	page2Desc.SetLineWrap(true)
	page2.PackStart(page2Desc, false, false, 0)

	regionList, err := gtk.ListBoxNew()
	if err != nil {
		return nil, err
	}
	regionList.SetSelectionMode(gtk.SELECTION_SINGLE)
	regionList.SetActivateOnSingleClick(false)
	page2.PackStart(regionList, true, true, 8)

	selectedRegionCheckboxes := uint8(0)

	europeRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	europeRow.SetSelectable(true)
	europeRow.SetActivatable(true)
	europeContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	applySetupRowStyle(europeContainer)
	europeRow.Add(europeContainer)

	europeCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	europeCheck.SetActive(true)
	selectedRegionCheckboxes++
	europeCheck.SetVAlign(gtk.ALIGN_CENTER)
	SetupCheckButtonAccessibility(europeCheck, "Include games from the European region")
	europeContainer.PackStart(europeCheck, false, false, 0)

	europeLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	europeLabel.SetMarkup("<span font='12' weight='600'>Europe</span>")
	europeLabel.SetHAlign(gtk.ALIGN_START)
	europeLabel.SetVAlign(gtk.ALIGN_CENTER)
	europeContainer.PackStart(europeLabel, true, true, 0)
	regionList.Add(europeRow)

	usaRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	usaRow.SetSelectable(true)
	usaRow.SetActivatable(true)
	usaContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	applySetupRowStyle(usaContainer)
	usaRow.Add(usaContainer)

	usaCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	usaCheck.SetActive(true)
	selectedRegionCheckboxes++
	usaCheck.SetVAlign(gtk.ALIGN_CENTER)
	SetupCheckButtonAccessibility(usaCheck, "Include games from the USA region")
	usaContainer.PackStart(usaCheck, false, false, 0)

	usaLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	usaLabel.SetMarkup("<span font='12' weight='600'>USA</span>")
	usaLabel.SetHAlign(gtk.ALIGN_START)
	usaLabel.SetVAlign(gtk.ALIGN_CENTER)
	usaContainer.PackStart(usaLabel, true, true, 0)
	regionList.Add(usaRow)

	japanRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	japanRow.SetSelectable(true)
	japanRow.SetActivatable(true)
	japanContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	applySetupRowStyle(japanContainer)
	japanRow.Add(japanContainer)

	japanCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	japanCheck.SetActive(true)
	selectedRegionCheckboxes++
	japanCheck.SetVAlign(gtk.ALIGN_CENTER)
	SetupCheckButtonAccessibility(japanCheck, "Include games from the Japan region")
	japanContainer.PackStart(japanCheck, false, false, 0)

	japanLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	japanLabel.SetMarkup("<span font='12' weight='600'>Japan</span>")
	japanLabel.SetHAlign(gtk.ALIGN_START)
	japanLabel.SetVAlign(gtk.ALIGN_CENTER)
	japanContainer.PackStart(japanLabel, true, true, 0)
	regionList.Add(japanRow)

	updateNextButton := func() {
		count := selectedCount(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive())
		nextButton.SetSensitive(count > 0)
		assistant.SetPageComplete(page2, count > 0)
	}

	europeCheck.Connect("toggled", updateNextButton)
	usaCheck.Connect("toggled", updateNextButton)
	japanCheck.Connect("toggled", updateNextButton)
	configureSetupOptionList(regionList,
		setupOptionRow{row: europeRow, check: europeCheck},
		setupOptionRow{row: usaRow, check: usaCheck},
		setupOptionRow{row: japanRow, check: japanCheck},
	)

	page3, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page3.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	page3.SetSpacing(SETUP_PAGE_SPACING)

	page3Label, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page3Label.SetMarkup("<span font='14' weight='bold'>Where do you want to play your games?</span>")
	page3Label.SetHAlign(gtk.ALIGN_START)
	page3.PackStart(page3Label, false, false, 0)

	page3Desc, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page3Desc.SetMarkup("<span font='11' alpha='80%'>Select one or both platforms. CEMU requires decryption, while Wii U keeps files encrypted for console use.</span>")
	page3Desc.SetHAlign(gtk.ALIGN_START)
	page3Desc.SetLineWrap(true)
	page3.PackStart(page3Desc, false, false, 0)

	platformList, err := gtk.ListBoxNew()
	if err != nil {
		return nil, err
	}
	platformList.SetSelectionMode(gtk.SELECTION_SINGLE)
	platformList.SetActivateOnSingleClick(false)
	page3.PackStart(platformList, true, true, 8)

	cemuRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	cemuRow.SetSelectable(true)
	cemuRow.SetActivatable(true)
	cemuOuterContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	applySetupRowStyle(cemuOuterContainer)
	cemuRow.Add(cemuOuterContainer)

	cemuCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	cemuCheck.SetActive(true)
	cemuCheck.SetVAlign(gtk.ALIGN_START)
	SetupCheckButtonAccessibility(cemuCheck, "Enable downloads for CEMU emulator with decryption")
	cemuOuterContainer.PackStart(cemuCheck, false, false, 0)

	cemuTextBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	cemuTextBox.SetSpacing(SETUP_SUB_TEXT_SPACING)

	cemuMainLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	cemuMainLabel.SetMarkup("<span font='12' weight='600'>CEMU - Emulator</span>")
	cemuMainLabel.SetHAlign(gtk.ALIGN_START)
	cemuTextBox.PackStart(cemuMainLabel, false, false, 0)

	cemuSubLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	cemuSubLabel.SetMarkup("<span font='10' alpha='80%'>Decrypt game files for use in the CEMU emulator</span>")
	cemuSubLabel.SetLineWrap(true)
	cemuSubLabel.SetHAlign(gtk.ALIGN_START)
	cemuTextBox.PackStart(cemuSubLabel, false, false, 0)

	cemuOuterContainer.PackStart(cemuTextBox, true, true, 0)
	platformList.Add(cemuRow)

	wiiURow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	wiiURow.SetSelectable(true)
	wiiURow.SetActivatable(true)
	wiiUOuterContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	applySetupRowStyle(wiiUOuterContainer)
	wiiURow.Add(wiiUOuterContainer)

	wiiUCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	wiiUCheck.SetActive(true)
	wiiUCheck.SetVAlign(gtk.ALIGN_START)
	SetupCheckButtonAccessibility(wiiUCheck, "Enable downloads for Wii U console with encrypted files")
	wiiUOuterContainer.PackStart(wiiUCheck, false, false, 0)

	wiiUTextBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	wiiUTextBox.SetSpacing(SETUP_SUB_TEXT_SPACING)

	wiiUMainLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	wiiUMainLabel.SetMarkup("<span font='12' weight='600'>Wii U Console</span>")
	wiiUMainLabel.SetHAlign(gtk.ALIGN_START)
	wiiUTextBox.PackStart(wiiUMainLabel, false, false, 0)

	wiiUSubLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	wiiUSubLabel.SetMarkup("<span font='10' alpha='80%'>Keep encrypted game files for installation on a Wii U console</span>")
	wiiUSubLabel.SetLineWrap(true)
	wiiUSubLabel.SetHAlign(gtk.ALIGN_START)
	wiiUTextBox.PackStart(wiiUSubLabel, false, false, 0)

	wiiUOuterContainer.PackStart(wiiUTextBox, true, true, 0)
	platformList.Add(wiiURow)
	configureSetupOptionList(platformList,
		setupOptionRow{row: cemuRow, check: cemuCheck},
		setupOptionRow{row: wiiURow, check: wiiUCheck},
	)
	updatePlatformSelection := func() {
		assistant.SetPageComplete(page3, cemuCheck.GetActive() || wiiUCheck.GetActive())
	}
	cemuCheck.Connect("toggled", updatePlatformSelection)
	wiiUCheck.Connect("toggled", updatePlatformSelection)

	// --- Storage Page ---
	pageStorage, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	pageStorage.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	pageStorage.SetSpacing(SETUP_PAGE_SPACING)

	pageStorageLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	pageStorageLabel.SetHAlign(gtk.ALIGN_START)
	pageStorage.PackStart(pageStorageLabel, false, false, 0)

	pageStorageDesc, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	pageStorageDesc.SetHAlign(gtk.ALIGN_START)
	pageStorageDesc.SetLineWrap(true)
	pageStorage.PackStart(pageStorageDesc, false, false, 0)

	downloadPathEntry, err := gtk.EntryNew()
	if err != nil {
		return nil, err
	}
	downloadPathEntry.SetPlaceholderText("Select download location...")
	downloadPathEntry.SetWidthChars(30)
	downloadPathEntry.SetHExpand(true)
	if config.LastSelectedPath != "" {
		downloadPathEntry.SetText(config.LastSelectedPath)
	}
	SetupEntryAccessibility(downloadPathEntry, "Download path", "Folder where downloaded game files will be saved.")

	decryptPathEntry, err := gtk.EntryNew()
	if err != nil {
		return nil, err
	}
	decryptPathEntry.SetPlaceholderText("Select decrypted game location...")
	decryptPathEntry.SetWidthChars(30)
	decryptPathEntry.SetHExpand(true)
	if config.DecryptOutputPath != "" {
		decryptPathEntry.SetText(config.DecryptOutputPath)
	}
	SetupEntryAccessibility(decryptPathEntry, "Decrypted output path", "Optional folder where decrypted game files will be saved. Leave empty to use the download location.")

	newPathRow := func(labelText string, entry *gtk.Entry, browseTitle string, clearLabel string) (*gtk.Box, error) {
		row, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
		if err != nil {
			return nil, err
		}
		row.SetMarginTop(8)

		label, err := gtk.LabelNew(labelText)
		if err != nil {
			return nil, err
		}
		label.SetHAlign(gtk.ALIGN_START)
		row.PackStart(label, false, false, 0)
		row.PackStart(entry, true, true, 0)

		browseButton, err := gtk.ButtonNewWithLabel("Browse")
		if err != nil {
			return nil, err
		}
		SetupButtonAccessibility(browseButton, "Browse for "+strings.ToLower(labelText))
		browseButton.Connect("clicked", func() {
			selectedPath, err := dialog.Directory().Title(browseTitle).Browse()
			if err == nil && selectedPath != "" {
				entry.SetText(selectedPath)
			}
		})

		clearButton, err := gtk.ButtonNewWithLabel(clearLabel)
		if err != nil {
			return nil, err
		}
		SetupButtonAccessibility(clearButton, "Clear "+strings.ToLower(labelText))
		addStyleClass(clearButton.GetStyleContext, "destructive-action")
		clearButton.Connect("clicked", func() {
			entry.SetText("")
		})

		buttonBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
		if err != nil {
			return nil, err
		}
		addStyleClass(buttonBox.GetStyleContext, "linked")
		buttonBox.PackStart(browseButton, true, true, 0)
		buttonBox.PackStart(clearButton, true, true, 0)
		row.PackStart(buttonBox, false, false, 0)
		return row, nil
	}

	downloadPathRow, err := newPathRow("Download path:", downloadPathEntry, "Select Download Path", "Clear")
	if err != nil {
		return nil, err
	}
	pageStorage.PackStart(downloadPathRow, false, false, 0)

	decryptPathRow, err := newPathRow("Decrypted path:", decryptPathEntry, "Select Decrypted Files Output Path", "Clear")
	if err != nil {
		return nil, err
	}
	pageStorage.PackStart(decryptPathRow, false, false, 0)

	updateStoragePage := func() {
		cemu := cemuCheck.GetActive()
		wiiU := wiiUCheck.GetActive()
		decryptOnly := cemu && !wiiU
		downloadPath, _ := downloadPathEntry.GetText()
		decryptPath, _ := decryptPathEntry.GetText()
		downloadPath = strings.TrimSpace(downloadPath)
		decryptPath = strings.TrimSpace(decryptPath)

		switch {
		case decryptOnly:
			pageStorageLabel.SetMarkup("<span font='14' weight='bold'>Where should decrypted games go?</span>")
			pageStorageDesc.SetMarkup("<span font='11' alpha='80%'>CEMU downloads are decrypted automatically. This folder will also be used as the regular download location.</span>")
			decryptPathEntry.SetPlaceholderText("Select decrypted game location...")
		case wiiU && !cemu:
			pageStorageLabel.SetMarkup("<span font='14' weight='bold'>Where should Wii U games go?</span>")
			decryptPathEntry.SetPlaceholderText("Same as download location...")
			pageStorageDesc.SetMarkup("<span font='11' alpha='80%'>Wii U downloads stay encrypted for installation on a console.</span>")
		default:
			pageStorageLabel.SetMarkup("<span font='14' weight='bold'>Where should games go?</span>")
			decryptPathEntry.SetPlaceholderText("Same as download location...")
			pageStorageDesc.SetMarkup("<span font='11' alpha='80%'>Choose a download folder and an optional separate folder for decrypted games. Leave decrypted path empty to use the download location.</span>")
		}

		downloadPathRow.SetVisible(!decryptOnly)
		decryptPathRow.SetVisible(cemu)
		storageComplete := (wiiU && downloadPath != "") || (decryptOnly && decryptPath != "") || (cemu && wiiU && downloadPath != "")
		assistant.SetPageComplete(pageStorage, storageComplete)
	}
	downloadPathEntry.Connect("changed", updateStoragePage)
	decryptPathEntry.Connect("changed", updateStoragePage)
	cemuCheck.Connect("toggled", updateStoragePage)
	wiiUCheck.Connect("toggled", updateStoragePage)

	storageSpacer, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	pageStorage.PackStart(storageSpacer, true, true, 0)

	// --- Finish Page ---
	page4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page4.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	page4.SetSpacing(SETUP_PAGE_SPACING_LARGE)

	assistant.AppendPage(page1)
	assistant.AppendPage(page2)
	assistant.AppendPage(page3)
	assistant.AppendPage(pageStorage)
	assistant.AppendPage(page4)

	page4Label, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page4Label.SetMarkup("<span font='18' weight='bold'>All Set!</span>")
	page4Label.SetHAlign(gtk.ALIGN_START)
	page4.PackStart(page4Label, false, false, 0)

	page4SubLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	page4SubLabel.SetMarkup("<span font='11' alpha='80%'>WiiUDownloader is now configured and ready to use. You can start downloading games immediately or adjust settings in the preferences menu.</span>")
	page4SubLabel.SetLineWrap(true)
	page4SubLabel.SetHAlign(gtk.ALIGN_START)
	page4.PackStart(page4SubLabel, false, false, 0)

	spacer4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page4.PackStart(spacer4, true, true, 0)

	summaryLabel, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	summaryLabel.SetMarkup("<span font='10' weight='600'>Configuration Summary:</span>")
	summaryLabel.SetHAlign(gtk.ALIGN_START)
	page4.PackStart(summaryLabel, false, false, 0)

	summaryBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	summaryBox.SetSpacing(SETUP_SUMMARY_SPACING)
	summaryBox.SetMarginTop(SETUP_SUMMARY_MARGIN)
	summaryBox.SetMarginStart(SETUP_SUMMARY_MARGIN)
	page4.PackStart(summaryBox, false, false, 0)

	summaryRegions, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	summaryRegions.SetMarkup("<span font='10' alpha='85%'>✓ Regions: Europe, USA, Japan</span>")
	summaryRegions.SetHAlign(gtk.ALIGN_START)
	summaryBox.PackStart(summaryRegions, false, false, 0)

	summaryPlatforms, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	summaryPlatforms.SetMarkup("<span font='10' alpha='85%'>✓ Platforms: CEMU + Wii U</span>")
	summaryPlatforms.SetHAlign(gtk.ALIGN_START)
	summaryBox.PackStart(summaryPlatforms, false, false, 0)

	summaryStorage, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	summaryStorage.SetMarkup("<span font='10' alpha='85%'>✓ Decrypt output: same as download</span>")
	summaryStorage.SetHAlign(gtk.ALIGN_START)
	summaryBox.PackStart(summaryStorage, false, false, 0)

	pages := []struct {
		widget *gtk.Box
		title  string
	}{
		{widget: page1, title: "Welcome"},
		{widget: page2, title: "Regions"},
		{widget: page3, title: "Platforms"},
		{widget: pageStorage, title: "Storage"},
		{widget: page4, title: "Finish"},
	}

	lastPageIndex := len(pages) - 1

	for _, p := range pages {
		assistant.SetPageComplete(p.widget, p.widget != pageStorage)
		assistant.SetPageType(p.widget, gtk.ASSISTANT_PAGE_CUSTOM)
		assistant.SetPageTitle(p.widget, p.title)
	}
	updatePlatformSelection()
	updateStoragePage()

	completeSetup := func() {
		config.DidInitialSetup = true
		selectedRegions := selectedRegionMask(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive())
		config.SelectedRegion = selectedRegions
		cemu := cemuCheck.GetActive()
		wiiU := wiiUCheck.GetActive()
		config.DecryptContents, config.DeleteEncryptedContents = platformSelectionToConfig(cemu, wiiU)
		downloadPath, _ := downloadPathEntry.GetText()
		decryptPath, _ := decryptPathEntry.GetText()
		config.LastSelectedPath, config.DecryptOutputPath = storagePathsForPlatforms(cemu, wiiU, downloadPath, decryptPath)

		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		closeAssistantWindow(assistant, performPostSetup)
	}

	assistant.Connect("apply", completeSetup)

	skipButton.Connect("clicked", func() {
		config.DidInitialSetup = true
		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		closeAssistantWindow(assistant, performPostSetup)
	})

	backButton.Connect("clicked", func() {
		assistant.SetCurrentPage(previousSetupPageIndex(assistant.GetCurrentPage()))
	})

	nextButton.Connect("clicked", func() {
		assistant.SetCurrentPage(nextSetupPageIndex(assistant.GetCurrentPage(), lastPageIndex))
	})

	finishButton.Connect("clicked", func() {
		completeSetup()
	})

	assistant.Connect("prepare", func(assistant *gtk.Assistant, page *gtk.Widget) {
		pageNum := assistant.GetCurrentPage()

		setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, false, false, false)

		isFinishPage := pageNum == 4

		if pageNum == 0 {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, true, false, true, false)
			nextButton.GrabFocus()
		} else if pageNum == 1 {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, true, true, false)
			count := selectedCount(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive())
			nextButton.SetSensitive(count > 0)
			focusSetupOptionList(regionList)
		} else if pageNum == 2 {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, true, true, false)
			nextButton.SetSensitive(true)
			focusSetupOptionList(platformList)
		} else if pageNum == 3 {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, true, true, false)
			nextButton.SetSensitive(true)
			if cemuCheck.GetActive() && !wiiUCheck.GetActive() {
				decryptPathEntry.GrabFocus()
			} else {
				downloadPathEntry.GrabFocus()
			}
		} else if isFinishPage {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, true, false, true)
			summaryRegions.SetMarkup("<span font='10' alpha='85%'>✓ Regions: " + selectedRegionsSummary(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive()) + "</span>")
			summaryPlatforms.SetMarkup("<span font='10' alpha='85%'>✓ Platforms: " + selectedPlatformsSummary(cemuCheck.GetActive(), wiiUCheck.GetActive()) + "</span>")
			cemu := cemuCheck.GetActive()
			wiiU := wiiUCheck.GetActive()
			downloadPath, _ := downloadPathEntry.GetText()
			decryptPath, _ := decryptPathEntry.GetText()
			lastPath, outputPath := storagePathsForPlatforms(cemu, wiiU, downloadPath, decryptPath)
			if cemu && !wiiU {
				summaryStorage.SetMarkup("<span font='10' alpha='85%'>✓ Games: " + escapeMarkup(lastPath) + "</span>")
			} else if wiiU && cemu {
				if outputPath == "" {
					outputPath = "same as download"
				}
				summaryStorage.SetMarkup("<span font='10' alpha='85%'>✓ Downloads: " + escapeMarkup(lastPath) + "\n✓ Decrypted: " + escapeMarkup(outputPath) + "</span>")
			} else {
				summaryStorage.SetMarkup("<span font='10' alpha='85%'>✓ Downloads: " + escapeMarkup(lastPath) + "</span>")
			}
			finishButton.GrabFocus()
		}
	})

	initialSetupAssistantWindow := InitialSetupAssistantWindow{
		assistantWindow:   assistant,
		config:            config,
		skipButton:        skipButton,
		nextButton:        nextButton,
		backButton:        backButton,
		finishButton:      finishButton,
		postSetupCallback: nil,
	}

	performPostSetup = func() {
		if initialSetupAssistantWindow.postSetupCallback != nil {
			initialSetupAssistantWindow.postSetupCallback()
		}
	}

	return &initialSetupAssistantWindow, nil
}

func platformSelectionToConfig(cemu, wiiU bool) (decryptContents, deleteEncryptedContents bool) {
	decryptContents = cemu
	deleteEncryptedContents = cemu && !wiiU
	return decryptContents, deleteEncryptedContents
}

func storagePathsForPlatforms(cemu, wiiU bool, downloadPath, decryptPath string) (lastSelectedPath, decryptOutputPath string) {
	if cemu && !wiiU {
		return decryptPath, ""
	}
	if wiiU && cemu {
		return downloadPath, decryptPath
	}
	return downloadPath, ""
}

type setupOptionRow struct {
	row   *gtk.ListBoxRow
	check *gtk.CheckButton
}

func configureSetupOptionList(list *gtk.ListBox, options ...setupOptionRow) {
	if list == nil {
		return
	}

	list.SetCanFocus(true)
	list.Connect("row-activated", func(_ *gtk.ListBox, row *gtk.ListBoxRow) {
		toggleSetupOptionForRow(row, options)
	})
	list.Connect("key-press-event", func(_ *gtk.ListBox, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		if !isKeyboardActivationKey(keyEvent.KeyVal()) {
			return false
		}

		row := list.GetSelectedRow()
		if row == nil {
			row = list.GetRowAtIndex(0)
			if row == nil {
				return false
			}
			list.SelectRow(row)
		}

		return toggleSetupOptionForRow(row, options)
	})

	for _, option := range options {
		if option.row == nil {
			continue
		}
		option.row.ToWidget().SetCanFocus(true)
	}
}

func toggleSetupOptionForRow(row *gtk.ListBoxRow, options []setupOptionRow) bool {
	if row == nil {
		return false
	}

	rowIndex := row.GetIndex()
	if rowIndex < 0 || rowIndex >= len(options) {
		return false
	}

	option := options[rowIndex]
	if option.check == nil {
		return false
	}

	option.check.SetActive(!option.check.GetActive())
	return true
}

func focusSetupOptionList(list *gtk.ListBox) {
	if list == nil {
		return
	}

	if list.GetSelectedRow() == nil {
		if firstRow := list.GetRowAtIndex(0); firstRow != nil {
			list.SelectRow(firstRow)
		}
	}
	list.GrabFocus()
}

func nextSetupPageIndex(currentPage, lastPageIndex int) int {
	if currentPage >= lastPageIndex {
		return lastPageIndex
	}
	if currentPage < 0 {
		return 0
	}
	return currentPage + 1
}

func previousSetupPageIndex(currentPage int) int {
	if currentPage <= 0 {
		return 0
	}
	return currentPage - 1
}

func (assistant *InitialSetupAssistantWindow) ShowAll() {
	assistant.assistantWindow.ShowAll()
}

func (assistant *InitialSetupAssistantWindow) Hide() {
	assistant.assistantWindow.Hide()
}

func (assistant *InitialSetupAssistantWindow) SetPostSetupCallback(cb func()) {
	assistant.postSetupCallback = cb
}

func selectedCount(flags ...bool) int {
	count := 0
	for _, flag := range flags {
		if flag {
			count++
		}
	}
	return count
}

func selectedRegionMask(europe, usa, japan bool) uint8 {
	selectedRegions := uint8(0)
	if europe {
		selectedRegions |= wiiudownloader.MCP_REGION_EUROPE
	}
	if usa {
		selectedRegions |= wiiudownloader.MCP_REGION_USA
	}
	if japan {
		selectedRegions |= wiiudownloader.MCP_REGION_JAPAN
	}
	return selectedRegions
}

func selectedRegionsSummary(europe, usa, japan bool) string {
	regions := ""
	if europe {
		regions += "Europe, "
	}
	if usa {
		regions += "USA, "
	}
	if japan {
		regions += "Japan, "
	}
	return strings.TrimRight(regions, ", ")
}

func selectedPlatformsSummary(cemu, wiiU bool) string {
	platforms := ""
	if cemu {
		platforms += "CEMU"
	}
	if wiiU {
		if platforms != "" {
			platforms += " + "
		}
		platforms += "Wii U"
	}
	return platforms
}

func setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton *gtk.Button, skip, back, next, finish bool) {
	skipButton.SetVisible(skip)
	backButton.SetVisible(back)
	nextButton.SetVisible(next)
	finishButton.SetVisible(finish)
}

func closeAssistantWindow(assistant *gtk.Assistant, callback func()) {
	assistant.Hide()
	if callback != nil {
		callback()
	}
	assistant.Emit("close", glib.TYPE_BOOLEAN, nil)
	assistant.SetDestroyWithParent(true)
}

func applySetupRowStyle(box *gtk.Box) {
	box.SetMarginStart(SETUP_ROW_HORIZONTAL_MARGIN)
	box.SetMarginEnd(SETUP_ROW_HORIZONTAL_MARGIN)
	box.SetMarginTop(SETUP_ROW_VERTICAL_MARGIN)
	box.SetMarginBottom(SETUP_ROW_VERTICAL_MARGIN)
	box.SetSpacing(SETUP_ROW_SPACING)
}
