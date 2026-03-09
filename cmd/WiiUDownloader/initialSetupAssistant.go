package main

import (
	"fmt"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
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
	info3.SetMarkup("<span font='10' alpha='80%'>▸ Review and confirm your configuration</span>")
	info3.SetHAlign(gtk.ALIGN_START)
	infoBox.PackStart(info3, false, false, 0)

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

	page4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page4.SetBorderWidth(SETUP_PAGE_BORDER_WIDTH)
	page4.SetSpacing(SETUP_PAGE_SPACING_LARGE)

	assistant.AppendPage(page1)
	assistant.AppendPage(page2)
	assistant.AppendPage(page3)
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

	pages := []struct {
		widget *gtk.Box
		title  string
	}{
		{widget: page1, title: "Welcome"},
		{widget: page2, title: "Regions"},
		{widget: page3, title: "Platforms"},
		{widget: page4, title: "Finish"},
	}

	lastPageIndex := len(pages) - 1

	for i, p := range pages {
		assistant.SetPageComplete(p.widget, true)
		assistant.SetPageType(p.widget, assistantPageTypeForIndex(i, len(pages)-1))
		assistant.SetPageTitle(p.widget, p.title)
	}

	completeSetup := func() {
		config.DidInitialSetup = true
		selectedRegions := selectedRegionMask(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive())
		config.SelectedRegion = selectedRegions
		config.DecryptContents, config.DeleteEncryptedContents = platformSelectionToConfig(cemuCheck.GetActive(), wiiUCheck.GetActive())

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

		isFinishPage := pageNum == 3

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
		} else if isFinishPage {
			setSetupButtonsVisible(skipButton, backButton, nextButton, finishButton, false, true, false, true)
			summaryRegions.SetMarkup("<span font='10' alpha='85%'>✓ Regions: " + selectedRegionsSummary(europeCheck.GetActive(), usaCheck.GetActive(), japanCheck.GetActive()) + "</span>")
			summaryPlatforms.SetMarkup("<span font='10' alpha='85%'>✓ Platforms: " + selectedPlatformsSummary(cemuCheck.GetActive(), wiiUCheck.GetActive()) + "</span>")
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

func assistantPageTypeForIndex(pageIndex, lastPageIndex int) gtk.AssistantPageType {
	if pageIndex == lastPageIndex {
		return gtk.ASSISTANT_PAGE_CUSTOM
	}
	return gtk.ASSISTANT_PAGE_CUSTOM
}

func platformSelectionToConfig(cemu, wiiU bool) (decryptContents, deleteEncryptedContents bool) {
	decryptContents = cemu
	deleteEncryptedContents = cemu && !wiiU
	return decryptContents, deleteEncryptedContents
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
	if len(regions) > 2 {
		return regions[:len(regions)-2]
	}
	return regions
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
