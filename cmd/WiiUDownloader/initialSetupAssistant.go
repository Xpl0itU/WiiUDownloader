package main

import (
	"fmt"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// InitialSetupAssistantWindow is the first-run wizard. GtkAssistant is deprecated
// with no GTK4 replacement, so the step bar and pages are hand-built.
type InitialSetupAssistantWindow struct {
	window            *gtk.Window
	adwWindow         *adw.Window
	headerBar         *adw.HeaderBar
	stack             *gtk.Stack
	stepList          *gtk.ListBox
	stepRows          []*gtk.ListBoxRow
	pageTitles        []string
	setPage           func(int)
	config            *Config
	skipButton        *gtk.Button
	nextButton        *gtk.Button
	backButton        *gtk.Button
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
	SETUP_STACK_TRANSITION_MS   = 180
)

func setupPageBox(spacing int) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetSpacing(spacing)
	box.SetMarginTop(SETUP_PAGE_BORDER_WIDTH)
	box.SetMarginBottom(SETUP_PAGE_BORDER_WIDTH)
	box.SetMarginStart(SETUP_PAGE_BORDER_WIDTH)
	box.SetMarginEnd(SETUP_PAGE_BORDER_WIDTH)
	return box
}

func NewInitialSetupAssistantWindow(config *Config) (*InitialSetupAssistantWindow, error) {
	var performPostSetup func()

	adwWin := adw.NewWindow()
	win := &adwWin.Window
	win.SetTitle(WINDOW_TITLE_PREFIX + "Initial Setup")
	win.SetDefaultSize(INITIAL_SETUP_WINDOW_WIDTH, INITIAL_SETUP_WINDOW_HEIGHT)
	win.SetModal(true)
	win.AddCSSClass("setup-wizard")

	headerBar := adw.NewHeaderBar()
	windowTitle := adw.NewWindowTitle("Initial Setup", "")
	headerBar.SetTitleWidget(windowTitle)

	// Identical button row on every step; only the primary button's label and
	// enabled state change.
	backButton := gtk.NewButtonWithLabel("Back")
	SetupButtonAccessibility(backButton, "Go back to the previous step")

	nextButton := gtk.NewButtonWithLabel("Next")
	SetupButtonAccessibility(nextButton, "Proceed to the next step")
	nextButton.AddCSSClass("suggested-action")

	skipButton := gtk.NewButtonWithLabel("Skip")
	skipButton.AddCSSClass("flat")
	SetupButtonAccessibility(skipButton, "Skip the initial setup wizard and start using the application with default settings")

	actionBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	actionBox.SetHAlign(gtk.AlignEnd)
	actionBox.Append(backButton)
	actionBox.Append(nextButton)

	actionRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	actionRow.AddCSSClass("setup-actions")
	actionRow.Append(skipButton)
	actionSpacer := gtk.NewLabel("")
	actionSpacer.SetHExpand(true)
	actionRow.Append(actionSpacer)
	actionRow.Append(actionBox)

	stack := gtk.NewStack()
	stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)
	stack.SetTransitionDuration(SETUP_STACK_TRANSITION_MS)
	stack.SetHExpand(true)
	stack.SetVExpand(true)

	// Only the page on screen may change Next, so completeness is stored here and
	// derived through refreshNext. Storage is deliberately always complete: an
	// unset path is asked for at the first download.
	complete := []bool{true, true, true, true, true}

	currentPage := 0
	refreshNext := func() {}

	page1 := setupPageBox(SETUP_PAGE_SPACING_LARGE)

	page1Label := gtk.NewLabel("")
	page1Label.SetMarkup("<span font='18' weight='bold'>Welcome to WiiUDownloader</span>")
	page1Label.SetHAlign(gtk.AlignStart)
	page1.Append(page1Label)

	page1SubLabel := gtk.NewLabel("")
	page1SubLabel.SetMarkup("<span font='11' alpha='85%'>This setup wizard will guide you through the initial configuration in just a few steps. You can modify these settings anytime later in the preferences.</span>")
	page1SubLabel.SetWrap(true)
	page1SubLabel.SetHAlign(gtk.AlignStart)
	page1.Append(page1SubLabel)

	spacer := gtk.NewBox(gtk.OrientationVertical, 0)
	spacer.SetVExpand(true)
	page1.Append(spacer)

	infoBox := gtk.NewBox(gtk.OrientationVertical, 0)
	infoBox.SetSpacing(SETUP_INFO_SPACING)

	info1 := gtk.NewLabel("")
	info1.SetMarkup("<span font='10' alpha='80%'>▸ Select your preferred game regions</span>")
	info1.SetHAlign(gtk.AlignStart)
	infoBox.Append(info1)

	info2 := gtk.NewLabel("")
	info2.SetMarkup("<span font='10' alpha='80%'>▸ Choose target platforms (emulator and/or console)</span>")
	info2.SetHAlign(gtk.AlignStart)
	infoBox.Append(info2)

	info3 := gtk.NewLabel("")
	info3.SetMarkup("<span font='10' alpha='80%'>▸ Set the storage location for decrypted game files</span>")
	info3.SetHAlign(gtk.AlignStart)
	infoBox.Append(info3)

	info4 := gtk.NewLabel("")
	info4.SetMarkup("<span font='10' alpha='80%'>▸ Review and confirm your configuration</span>")
	info4.SetHAlign(gtk.AlignStart)
	infoBox.Append(info4)

	page1.Append(infoBox)

	page2 := setupPageBox(SETUP_PAGE_SPACING)

	page2Label := gtk.NewLabel("")
	page2Label.SetMarkup("<span font='14' weight='bold'>Which regions do you want to download from?</span>")
	page2Label.SetHAlign(gtk.AlignStart)
	page2.Append(page2Label)

	page2Desc := gtk.NewLabel("")
	page2Desc.SetMarkup("<span font='11' alpha='80%'>Select one or more regions to enable downloading games from their respective game libraries.</span>")
	page2Desc.SetHAlign(gtk.AlignStart)
	page2Desc.SetWrap(true)
	page2.Append(page2Desc)

	regionList := gtk.NewListBox()
	regionList.SetSelectionMode(gtk.SelectionSingle)
	regionList.SetActivateOnSingleClick(false)
	regionList.SetVExpand(true)
	regionList.SetMarginTop(8)
	page2.Append(regionList)

	selectedRegionCheckboxes := uint8(0)

	europeRow := gtk.NewListBoxRow()
	europeRow.SetSelectable(true)
	europeRow.SetActivatable(true)
	europeContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	applySetupRowStyle(europeContainer)
	europeRow.SetChild(europeContainer)

	europeCheck := gtk.NewCheckButtonWithLabel("")
	europeCheck.SetActive(true)
	selectedRegionCheckboxes++
	europeCheck.SetVAlign(gtk.AlignCenter)
	SetupCheckButtonAccessibility(europeCheck, "Include games from the European region")
	europeContainer.Append(europeCheck)

	europeLabel := gtk.NewLabel("")
	europeLabel.SetMarkup("<span font='12' weight='600'>Europe</span>")
	europeLabel.SetHAlign(gtk.AlignStart)
	europeLabel.SetVAlign(gtk.AlignCenter)
	europeLabel.SetHExpand(true)
	europeContainer.Append(europeLabel)
	regionList.Append(europeRow)

	usaRow := gtk.NewListBoxRow()
	usaRow.SetSelectable(true)
	usaRow.SetActivatable(true)
	usaContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	applySetupRowStyle(usaContainer)
	usaRow.SetChild(usaContainer)

	usaCheck := gtk.NewCheckButtonWithLabel("")
	usaCheck.SetActive(true)
	selectedRegionCheckboxes++
	usaCheck.SetVAlign(gtk.AlignCenter)
	SetupCheckButtonAccessibility(usaCheck, "Include games from the USA region")
	usaContainer.Append(usaCheck)

	usaLabel := gtk.NewLabel("")
	usaLabel.SetMarkup("<span font='12' weight='600'>USA</span>")
	usaLabel.SetHAlign(gtk.AlignStart)
	usaLabel.SetVAlign(gtk.AlignCenter)
	usaLabel.SetHExpand(true)
	usaContainer.Append(usaLabel)
	regionList.Append(usaRow)

	japanRow := gtk.NewListBoxRow()
	japanRow.SetSelectable(true)
	japanRow.SetActivatable(true)
	japanContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	applySetupRowStyle(japanContainer)
	japanRow.SetChild(japanContainer)

	japanCheck := gtk.NewCheckButtonWithLabel("")
	japanCheck.SetActive(true)
	selectedRegionCheckboxes++
	japanCheck.SetVAlign(gtk.AlignCenter)
	SetupCheckButtonAccessibility(japanCheck, "Include games from the Japan region")
	japanContainer.Append(japanCheck)

	japanLabel := gtk.NewLabel("")
	japanLabel.SetMarkup("<span font='12' weight='600'>Japan</span>")
	japanLabel.SetHAlign(gtk.AlignStart)
	japanLabel.SetVAlign(gtk.AlignCenter)
	japanLabel.SetHExpand(true)
	japanContainer.Append(japanLabel)
	regionList.Append(japanRow)

	updateNextButton := func() {
		complete[1] = selectedCount(europeCheck.Active(), usaCheck.Active(), japanCheck.Active()) > 0
		refreshNext()
	}

	europeCheck.ConnectToggled(updateNextButton)
	usaCheck.ConnectToggled(updateNextButton)
	japanCheck.ConnectToggled(updateNextButton)
	configureSetupOptionList(regionList,
		setupOptionRow{row: europeRow, check: europeCheck},
		setupOptionRow{row: usaRow, check: usaCheck},
		setupOptionRow{row: japanRow, check: japanCheck},
	)

	page3 := setupPageBox(SETUP_PAGE_SPACING)

	page3Label := gtk.NewLabel("")
	page3Label.SetMarkup("<span font='14' weight='bold'>Where do you want to play your games?</span>")
	page3Label.SetHAlign(gtk.AlignStart)
	page3.Append(page3Label)

	page3Desc := gtk.NewLabel("")
	page3Desc.SetMarkup("<span font='11' alpha='80%'>Select one or both platforms. CEMU requires decryption, while Wii U keeps files encrypted for console use.</span>")
	page3Desc.SetHAlign(gtk.AlignStart)
	page3Desc.SetWrap(true)
	page3.Append(page3Desc)

	platformList := gtk.NewListBox()
	platformList.SetSelectionMode(gtk.SelectionSingle)
	platformList.SetActivateOnSingleClick(false)
	platformList.SetVExpand(true)
	platformList.SetMarginTop(8)
	page3.Append(platformList)

	cemuRow := gtk.NewListBoxRow()
	cemuRow.SetSelectable(true)
	cemuRow.SetActivatable(true)
	cemuOuterContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	applySetupRowStyle(cemuOuterContainer)
	cemuRow.SetChild(cemuOuterContainer)

	cemuCheck := gtk.NewCheckButtonWithLabel("")
	cemuCheck.SetActive(true)
	cemuCheck.SetVAlign(gtk.AlignStart)
	SetupCheckButtonAccessibility(cemuCheck, "Enable downloads for CEMU emulator with decryption")
	cemuOuterContainer.Append(cemuCheck)

	cemuTextBox := gtk.NewBox(gtk.OrientationVertical, 0)
	cemuTextBox.SetSpacing(SETUP_SUB_TEXT_SPACING)
	cemuTextBox.SetHExpand(true)

	cemuMainLabel := gtk.NewLabel("")
	cemuMainLabel.SetMarkup("<span font='12' weight='600'>CEMU - Emulator</span>")
	cemuMainLabel.SetHAlign(gtk.AlignStart)
	cemuTextBox.Append(cemuMainLabel)

	cemuSubLabel := gtk.NewLabel("")
	cemuSubLabel.SetMarkup("<span font='10' alpha='80%'>Decrypt game files for use in the CEMU emulator</span>")
	cemuSubLabel.SetWrap(true)
	cemuSubLabel.SetHAlign(gtk.AlignStart)
	cemuTextBox.Append(cemuSubLabel)

	cemuOuterContainer.Append(cemuTextBox)
	platformList.Append(cemuRow)

	wiiURow := gtk.NewListBoxRow()
	wiiURow.SetSelectable(true)
	wiiURow.SetActivatable(true)
	wiiUOuterContainer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	applySetupRowStyle(wiiUOuterContainer)
	wiiURow.SetChild(wiiUOuterContainer)

	wiiUCheck := gtk.NewCheckButtonWithLabel("")
	wiiUCheck.SetActive(true)
	wiiUCheck.SetVAlign(gtk.AlignStart)
	SetupCheckButtonAccessibility(wiiUCheck, "Enable downloads for Wii U console with encrypted files")
	wiiUOuterContainer.Append(wiiUCheck)

	wiiUTextBox := gtk.NewBox(gtk.OrientationVertical, 0)
	wiiUTextBox.SetSpacing(SETUP_SUB_TEXT_SPACING)
	wiiUTextBox.SetHExpand(true)

	wiiUMainLabel := gtk.NewLabel("")
	wiiUMainLabel.SetMarkup("<span font='12' weight='600'>Wii U Console</span>")
	wiiUMainLabel.SetHAlign(gtk.AlignStart)
	wiiUTextBox.Append(wiiUMainLabel)

	wiiUSubLabel := gtk.NewLabel("")
	wiiUSubLabel.SetMarkup("<span font='10' alpha='80%'>Keep encrypted game files for installation on a Wii U console</span>")
	wiiUSubLabel.SetWrap(true)
	wiiUSubLabel.SetHAlign(gtk.AlignStart)
	wiiUTextBox.Append(wiiUSubLabel)

	wiiUOuterContainer.Append(wiiUTextBox)
	platformList.Append(wiiURow)
	configureSetupOptionList(platformList,
		setupOptionRow{row: cemuRow, check: cemuCheck},
		setupOptionRow{row: wiiURow, check: wiiUCheck},
	)
	updatePlatformSelection := func() {
		complete[2] = cemuCheck.Active() || wiiUCheck.Active()
		refreshNext()
	}
	cemuCheck.ConnectToggled(updatePlatformSelection)
	wiiUCheck.ConnectToggled(updatePlatformSelection)

	// --- Storage Page ---
	pageStorage := setupPageBox(SETUP_PAGE_SPACING)

	pageStorageLabel := gtk.NewLabel("")
	pageStorageLabel.SetHAlign(gtk.AlignStart)
	pageStorage.Append(pageStorageLabel)

	pageStorageDesc := gtk.NewLabel("")
	pageStorageDesc.SetHAlign(gtk.AlignStart)
	pageStorageDesc.SetWrap(true)
	pageStorage.Append(pageStorageDesc)

	downloadPathEntry := gtk.NewEntry()
	downloadPathEntry.SetPlaceholderText("Select download location...")
	downloadPathEntry.SetWidthChars(30)
	downloadPathEntry.SetHExpand(true)
	if config.LastSelectedPath != "" {
		downloadPathEntry.SetText(config.LastSelectedPath)
	}
	SetupEntryAccessibility(downloadPathEntry, "Download path", "Folder where downloaded game files will be saved.")

	decryptPathEntry := gtk.NewEntry()
	decryptPathEntry.SetPlaceholderText("Select decrypted game location...")
	decryptPathEntry.SetWidthChars(30)
	decryptPathEntry.SetHExpand(true)
	if config.DecryptOutputPath != "" {
		decryptPathEntry.SetText(config.DecryptOutputPath)
	}
	SetupEntryAccessibility(decryptPathEntry, "Decrypted output path", "Optional folder where decrypted game files will be saved. Leave empty to use the download location.")

	// Keeps both row labels the same width so the two entries line up.
	pathLabelGroup := gtk.NewSizeGroup(gtk.SizeGroupHorizontal)

	newPathRow := func(labelText string, entry *gtk.Entry, browseTitle string, clearLabel string) *gtk.Box {
		row := gtk.NewBox(gtk.OrientationHorizontal, 6)
		row.SetMarginTop(8)

		label := gtk.NewLabel(labelText)
		label.SetHAlign(gtk.AlignStart)
		pathLabelGroup.AddWidget(label)
		row.Append(label)
		entry.SetHExpand(true)
		row.Append(entry)

		pathName := strings.ToLower(strings.TrimSuffix(labelText, ":"))

		browseButton := gtk.NewButtonWithLabel("Browse")
		SetupButtonAccessibility(browseButton, "Browse for "+pathName)
		browseButton.ConnectClicked(func() {
			chooseFolder(win, browseTitle, "", func(selectedPath string) {
				if selectedPath != "" {
					entry.SetText(selectedPath)
				}
			})
		})

		clearButton := gtk.NewButtonWithLabel(clearLabel)
		SetupButtonAccessibility(clearButton, "Clear "+pathName)
		clearButton.AddCSSClass("destructive-action")
		clearButton.ConnectClicked(func() {
			entry.SetText("")
		})

		buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
		buttonBox.AddCSSClass("linked")
		buttonBox.Append(browseButton)
		buttonBox.Append(clearButton)
		row.Append(buttonBox)
		return row
	}

	downloadPathRow := newPathRow("Download path:", downloadPathEntry, WINDOW_TITLE_PREFIX+"Select Download Path", "Clear")
	pageStorage.Append(downloadPathRow)

	decryptPathRow := newPathRow("Decrypted output path:", decryptPathEntry, WINDOW_TITLE_PREFIX+"Select Decrypted Output Path", "Clear")
	pageStorage.Append(decryptPathRow)

	updateStoragePage := func() {
		cemu := cemuCheck.Active()
		wiiU := wiiUCheck.Active()
		decryptOnly := cemu && !wiiU

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
	}
	downloadPathEntry.ConnectChanged(updateStoragePage)
	decryptPathEntry.ConnectChanged(updateStoragePage)
	cemuCheck.ConnectToggled(updateStoragePage)
	wiiUCheck.ConnectToggled(updateStoragePage)

	storageSpacer := gtk.NewBox(gtk.OrientationVertical, 0)
	storageSpacer.SetVExpand(true)
	pageStorage.Append(storageSpacer)

	// --- Finish Page ---
	page4 := setupPageBox(SETUP_PAGE_SPACING_LARGE)

	page4Label := gtk.NewLabel("")
	page4Label.SetMarkup("<span font='18' weight='bold'>All Set!</span>")
	page4Label.SetHAlign(gtk.AlignStart)
	page4.Append(page4Label)

	page4SubLabel := gtk.NewLabel("")
	page4SubLabel.SetMarkup("<span font='11' alpha='80%'>WiiUDownloader is now configured and ready to use. You can start downloading games immediately or adjust settings in the preferences menu.</span>")
	page4SubLabel.SetWrap(true)
	page4SubLabel.SetHAlign(gtk.AlignStart)
	page4.Append(page4SubLabel)

	spacer4 := gtk.NewBox(gtk.OrientationVertical, 0)
	spacer4.SetVExpand(true)
	page4.Append(spacer4)

	summaryLabel := gtk.NewLabel("")
	summaryLabel.SetMarkup("<span font='10' weight='600'>Configuration Summary:</span>")
	summaryLabel.SetHAlign(gtk.AlignStart)
	page4.Append(summaryLabel)

	summaryBox := gtk.NewBox(gtk.OrientationVertical, 0)
	summaryBox.SetSpacing(SETUP_SUMMARY_SPACING)
	summaryBox.SetMarginTop(SETUP_SUMMARY_MARGIN)
	summaryBox.SetMarginStart(SETUP_SUMMARY_MARGIN)
	page4.Append(summaryBox)

	summaryRegions := gtk.NewLabel("")
	summaryRegions.SetMarkup("<span font='10' alpha='85%'>✓ Regions: Europe, USA, Japan</span>")
	summaryRegions.SetHAlign(gtk.AlignStart)
	summaryBox.Append(summaryRegions)

	summaryPlatforms := gtk.NewLabel("")
	summaryPlatforms.SetMarkup("<span font='10' alpha='85%'>✓ Platforms: CEMU + Wii U</span>")
	summaryPlatforms.SetHAlign(gtk.AlignStart)
	summaryBox.Append(summaryPlatforms)

	summaryDownloads := gtk.NewLabel("")
	summaryDownloads.SetMarkup("<span font='10' alpha='85%'>✓ Downloads: same as download</span>")
	summaryDownloads.SetHAlign(gtk.AlignStart)
	summaryBox.Append(summaryDownloads)

	summaryDecrypted := gtk.NewLabel("")
	summaryDecrypted.SetHAlign(gtk.AlignStart)
	summaryDecrypted.SetVisible(false)
	summaryBox.Append(summaryDecrypted)

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
		stack.AddNamed(p.widget, p.title)
	}

	stepList := gtk.NewListBox()
	stepList.AddCSSClass("navigation-sidebar")
	stepList.AddCSSClass("setup-sidebar")
	stepList.SetSelectionMode(gtk.SelectionSingle)
	stepList.SetActivateOnSingleClick(true)
	SetupListViewAccessibility(stepList)

	// A size group keeps every title as wide as the widest one, so the sidebar
	// cannot grow as "current step" moves between pages.
	stepLabelGroup := gtk.NewSizeGroup(gtk.SizeGroupHorizontal)

	stepRows := make([]*gtk.ListBoxRow, 0, len(pages))
	for i, page := range pages {
		row := gtk.NewListBoxRow()
		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 10)
		rowBox.AddCSSClass("setup-step-row")
		badge := gtk.NewLabel(fmt.Sprintf("%d", i+1))
		badge.AddCSSClass("setup-step-badge")
		label := gtk.NewLabel(page.title)
		label.SetHAlign(gtk.AlignStart)
		label.SetHExpand(true)
		stepLabelGroup.AddWidget(label)
		rowBox.Append(badge)
		rowBox.Append(label)
		row.SetChild(rowBox)
		stepList.Append(row)
		stepRows = append(stepRows, row)
	}

	sidebar := gtk.NewBox(gtk.OrientationVertical, 0)
	sidebar.AddCSSClass("setup-sidebar-box")
	sidebar.Append(stepList)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.SetHExpand(true)
	contentBox.SetVExpand(true)
	contentBox.Append(stack)

	body := gtk.NewBox(gtk.OrientationHorizontal, 0)
	body.SetVExpand(true)
	body.Append(sidebar)
	body.Append(contentBox)

	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.Append(body)
	root.Append(actionRow)

	view := adw.NewToolbarView()
	view.AddTopBar(headerBar)
	view.SetContent(root)
	adwWin.SetContent(view)

	showPage := func(index int) {}
	applyTitle := func(index int) {
		pageTitle := "Initial Setup"
		if index >= 0 && index < len(pages) {
			pageTitle = pages[index].title
		}
		win.SetTitle(WINDOW_TITLE_PREFIX + pageTitle)
		windowTitle.SetTitle(pageTitle)
		windowTitle.SetSubtitle(fmt.Sprintf("Step %d of %d", index+1, len(pages)))
	}
	updatePlatformSelection()
	updateStoragePage()

	completeSetup := func() {
		config.DidInitialSetup = true
		selectedRegions := selectedRegionMask(europeCheck.Active(), usaCheck.Active(), japanCheck.Active())
		config.SelectedRegion = selectedRegions
		cemu := cemuCheck.Active()
		wiiU := wiiUCheck.Active()
		config.DecryptContents, config.DeleteEncryptedContents = platformSelectionToConfig(cemu, wiiU)
		config.LastSelectedPath, config.DecryptOutputPath = storagePathsForPlatforms(cemu, wiiU, downloadPathEntry.Text(), decryptPathEntry.Text())

		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		closeAssistantWindow(win, performPostSetup)
	}

	skipButton.ConnectClicked(func() {
		config.DidInitialSetup = true
		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		closeAssistantWindow(win, performPostSetup)
	})

	backButton.ConnectClicked(func() {
		showPage(previousSetupPageIndex(currentPage))
	})

	nextButton.ConnectClicked(func() {
		if currentPage >= lastPageIndex {
			completeSetup()
			return
		}
		showPage(nextSetupPageIndex(currentPage, lastPageIndex))
	})

	for index, row := range stepRows {
		step := index
		stepList.ConnectRowActivated(func(r *gtk.ListBoxRow) {
			if r == nil || step == currentPage {
				return
			}
			for earlier := 0; earlier < step; earlier++ {
				if !complete[earlier] {
					stepList.SelectRow(stepRows[currentPage])
					return
				}
			}
			showPage(step)
		})
		_ = row
	}

	showPage = func(index int) {
		if index < 0 || index >= len(pages) {
			index = currentPage
		}
		pageNum := index
		currentPage = index
		stack.SetVisibleChildName(pages[index].title)
		applyTitle(index)
		stepList.SelectRow(stepRows[index])

		finishStep := pageNum >= len(pages)-1
		nextButton.SetLabel("Next")
		if finishStep {
			nextButton.SetLabel("Finish")
		}
		backButton.SetSensitive(pageNum > 0)
		refreshNext()

		switch {
		case pageNum == 0:
			nextButton.GrabFocus()
		case pageNum == 1:
			focusSetupOptionList(regionList)
		case pageNum == 2:
			focusSetupOptionList(platformList)
		case pageNum == 3:
			if cemuCheck.Active() && !wiiUCheck.Active() {
				decryptPathEntry.GrabFocus()
			} else {
				downloadPathEntry.GrabFocus()
			}
		case finishStep:
			setSummaryLabel(summaryRegions, "✓ Regions: ", selectedRegionsSummary(europeCheck.Active(), usaCheck.Active(), japanCheck.Active()))
			setSummaryLabel(summaryPlatforms, "✓ Platforms: ", selectedPlatformsSummary(cemuCheck.Active(), wiiUCheck.Active()))
			cemu := cemuCheck.Active()
			wiiU := wiiUCheck.Active()
			lastPath, outputPath := storagePathsForPlatforms(cemu, wiiU, downloadPathEntry.Text(), decryptPathEntry.Text())
			lastPath = strings.TrimSpace(lastPath)
			outputPath = strings.TrimSpace(outputPath)
			if cemu && !wiiU {
				setSummaryLabel(summaryDownloads, "✓ Games: ", lastPath)
			} else {
				setSummaryLabel(summaryDownloads, "✓ Downloads: ", lastPath)
			}
			summaryDecrypted.SetVisible(false)
			if cemu && wiiU {
				decryptedPath := outputPath
				if decryptedPath == "" && lastPath != "" {
					decryptedPath = "same as download"
				}
				setSummaryLabel(summaryDecrypted, "✓ Decrypted: ", decryptedPath)
			}
			nextButton.GrabFocus()
		}
	}

	// Derived, never poked directly: a handler on a hidden page must not touch the
	// button of the page on screen.
	refreshNext = func() {
		nextButton.SetSensitive(currentPage >= lastPageIndex || complete[currentPage])
	}

	initialSetupAssistantWindow := InitialSetupAssistantWindow{
		window:            win,
		adwWindow:         adwWin,
		headerBar:         headerBar,
		stack:             stack,
		stepList:          stepList,
		stepRows:          stepRows,
		pageTitles:        pageTitles(pages),
		setPage:           showPage,
		config:            config,
		skipButton:        skipButton,
		nextButton:        nextButton,
		backButton:        backButton,
		postSetupCallback: nil,
	}

	performPostSetup = func() {
		if initialSetupAssistantWindow.postSetupCallback != nil {
			initialSetupAssistantWindow.postSetupCallback()
		}
	}

	// GtkStack shows its first child but leaves the sidebar and button states
	// untouched, so the wizard must select step 0 explicitly.
	initialSetupAssistantWindow.setPage(0)

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
	list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		toggleSetupOptionForRow(row, options)
	})

	keyController := gtk.NewEventControllerKey()
	keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if !isKeyboardActivationKey(keyval) {
			return false
		}

		row := list.SelectedRow()
		if row == nil {
			row = list.RowAtIndex(0)
			if row == nil {
				return false
			}
			list.SelectRow(row)
		}

		return toggleSetupOptionForRow(row, options)
	})
	list.AddController(keyController)

	for _, option := range options {
		if option.row == nil {
			continue
		}
		option.row.SetCanFocus(true)
	}
}

func toggleSetupOptionForRow(row *gtk.ListBoxRow, options []setupOptionRow) bool {
	if row == nil {
		return false
	}

	rowIndex := row.Index()
	if rowIndex < 0 || rowIndex >= len(options) {
		return false
	}

	option := options[rowIndex]
	if option.check == nil {
		return false
	}

	option.check.SetActive(!option.check.Active())
	return true
}

func focusSetupOptionList(list *gtk.ListBox) {
	if list == nil {
		return
	}

	if list.SelectedRow() == nil {
		if firstRow := list.RowAtIndex(0); firstRow != nil {
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
	assistant.window.Present()
}

func (assistant *InitialSetupAssistantWindow) Hide() {
	assistant.window.SetVisible(false)
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

func setSummaryLabel(label *gtk.Label, prefix, value string) {
	if value == "" {
		label.SetVisible(false)
		return
	}
	label.SetMarkup("<span font='10' alpha='85%'>" + prefix + glib.MarkupEscapeText(value) + "</span>")
	label.SetVisible(true)
}

func closeAssistantWindow(win *gtk.Window, callback func()) {
	win.SetVisible(false)
	if callback != nil {
		callback()
	}
}

func pageTitles(pages []struct {
	widget *gtk.Box
	title  string
}) []string {
	titles := make([]string, 0, len(pages))
	for _, p := range pages {
		titles = append(titles, p.title)
	}
	return titles
}

func applySetupRowStyle(box *gtk.Box) {
	box.SetMarginStart(SETUP_ROW_HORIZONTAL_MARGIN)
	box.SetMarginEnd(SETUP_ROW_HORIZONTAL_MARGIN)
	box.SetMarginTop(SETUP_ROW_VERTICAL_MARGIN)
	box.SetMarginBottom(SETUP_ROW_VERTICAL_MARGIN)
	box.SetSpacing(SETUP_ROW_SPACING)
}
