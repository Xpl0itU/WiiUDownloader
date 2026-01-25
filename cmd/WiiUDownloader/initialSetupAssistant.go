package main

import (
	"fmt"
	"os"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

type InitialSetupAssistantWindow struct {
	assistantWindow *gtk.Assistant
	config          *Config
	skipButton      *gtk.Button
}

func NewInitialSetupAssistantWindow(config *Config) (*InitialSetupAssistantWindow, error) {
	assistant, err := gtk.AssistantNew()
	if err != nil {
		return nil, err
	}
	assistant.Connect("cancel", func() {
		os.Exit(0) // Hacky way to close the program
	})
	assistant.SetTitle("WiiUDownloader - Initial Setup")
	assistant.SetDefaultSize(600, 500)
	assistant.SetPosition(gtk.WIN_POS_CENTER)
	assistant.SetKeepAbove(true)

	page1, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page1.SetBorderWidth(24)
	page1.SetSpacing(16)
	assistant.AppendPage(page1)

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

	// Spacing
	spacer, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page1.PackStart(spacer, true, true, 0)

	// Info items
	infoBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	infoBox.SetSpacing(8)

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

	// Skip button
	skipButton, err := gtk.ButtonNewWithLabel("Skip")
	if err != nil {
		return nil, err
	}
	assistant.AddActionWidget(skipButton)
	// Accessibility: Set button description for screen readers
	SetupButtonAccessibility(skipButton, "Skip the initial setup wizard and start using the application with default settings")
	skipButton.Connect("clicked", func() {
		config.DidInitialSetup = true
		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		assistant.Hide()
		assistant.Emit("close", glib.TYPE_BOOLEAN, nil)
		assistant.SetDestroyWithParent(true)
	})

	page2, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page2.SetBorderWidth(24)
	page2.SetSpacing(12)
	assistant.AppendPage(page2)

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
	regionList.SetSelectionMode(gtk.SELECTION_NONE)
	regionList.SetActivateOnSingleClick(false)
	page2.PackStart(regionList, true, true, 8)

	selectedRegionCheckboxes := uint8(0)

	// Europe
	europeRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	europeRow.SetSelectable(false)
	europeContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	europeContainer.SetMarginStart(16)
	europeContainer.SetMarginEnd(16)
	europeContainer.SetMarginTop(12)
	europeContainer.SetMarginBottom(12)
	europeContainer.SetSpacing(12)
	europeRow.Add(europeContainer)

	europeCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	europeCheck.SetActive(true)
	selectedRegionCheckboxes++
	europeCheck.SetVAlign(gtk.ALIGN_CENTER)
	// Accessibility: Set checkbox description
	SetupCheckButtonAccessibility(europeCheck, "Include games from the European region")
	europeCheck.Connect("toggled", func() {
		if europeCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
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

	// USA
	usaRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	usaRow.SetSelectable(false)
	usaContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	usaContainer.SetMarginStart(16)
	usaContainer.SetMarginEnd(16)
	usaContainer.SetMarginTop(12)
	usaContainer.SetMarginBottom(12)
	usaContainer.SetSpacing(12)
	usaRow.Add(usaContainer)

	usaCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	usaCheck.SetActive(true)
	selectedRegionCheckboxes++
	usaCheck.SetVAlign(gtk.ALIGN_CENTER)
	// Accessibility: Set checkbox description
	SetupCheckButtonAccessibility(usaCheck, "Include games from the USA region")
	usaCheck.Connect("toggled", func() {
		if usaCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
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

	// Japan
	japanRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	japanRow.SetSelectable(false)
	japanContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	japanContainer.SetMarginStart(16)
	japanContainer.SetMarginEnd(16)
	japanContainer.SetMarginTop(12)
	japanContainer.SetMarginBottom(12)
	japanContainer.SetSpacing(12)
	japanRow.Add(japanContainer)

	japanCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	japanCheck.SetActive(true)
	selectedRegionCheckboxes++
	japanCheck.SetVAlign(gtk.ALIGN_CENTER)
	// Accessibility: Set checkbox description
	SetupCheckButtonAccessibility(japanCheck, "Include games from the Japan region")
	japanCheck.Connect("toggled", func() {
		if japanCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
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

	page3, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page3.SetBorderWidth(24)
	page3.SetSpacing(12)
	assistant.AppendPage(page3)

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
	platformList.SetSelectionMode(gtk.SELECTION_NONE)
	platformList.SetActivateOnSingleClick(false)
	page3.PackStart(platformList, true, true, 8)

	// CEMU
	cemuRow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	cemuRow.SetSelectable(false)
	cemuOuterContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	cemuOuterContainer.SetMarginStart(16)
	cemuOuterContainer.SetMarginEnd(16)
	cemuOuterContainer.SetMarginTop(12)
	cemuOuterContainer.SetMarginBottom(12)
	cemuOuterContainer.SetSpacing(12)
	cemuRow.Add(cemuOuterContainer)

	cemuCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	cemuCheck.SetActive(true)
	cemuCheck.SetVAlign(gtk.ALIGN_START)
	// Accessibility: Set checkbox description
	SetupCheckButtonAccessibility(cemuCheck, "Enable downloads for CEMU emulator with decryption")
	cemuOuterContainer.PackStart(cemuCheck, false, false, 0)

	cemuTextBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	cemuTextBox.SetSpacing(2)

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

	// Wii U
	wiiURow, err := gtk.ListBoxRowNew()
	if err != nil {
		return nil, err
	}
	wiiURow.SetSelectable(false)
	wiiUOuterContainer, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	wiiUOuterContainer.SetMarginStart(16)
	wiiUOuterContainer.SetMarginEnd(16)
	wiiUOuterContainer.SetMarginTop(12)
	wiiUOuterContainer.SetMarginBottom(12)
	wiiUOuterContainer.SetSpacing(12)
	wiiURow.Add(wiiUOuterContainer)

	wiiUCheck, err := gtk.CheckButtonNewWithLabel("")
	if err != nil {
		return nil, err
	}
	wiiUCheck.SetActive(true)
	wiiUCheck.SetVAlign(gtk.ALIGN_START)
	// Accessibility: Set checkbox description
	SetupCheckButtonAccessibility(wiiUCheck, "Enable downloads for Wii U console with encrypted files")
	wiiUOuterContainer.PackStart(wiiUCheck, false, false, 0)

	wiiUTextBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	wiiUTextBox.SetSpacing(2)

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

	page4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0) // Finish page
	if err != nil {
		return nil, err
	}
	page4.SetBorderWidth(24)
	page4.SetSpacing(16)
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

	// Spacing
	spacer4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page4.PackStart(spacer4, true, true, 0)

	// Summary box
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
	summaryBox.SetSpacing(4)
	summaryBox.SetMarginTop(8)
	summaryBox.SetMarginStart(8)
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

	assistant.SetPageComplete(page1, true)
	assistant.SetPageComplete(page2, true)
	assistant.SetPageComplete(page3, true)
	assistant.SetPageComplete(page4, true)

	assistant.SetPageType(page1, gtk.ASSISTANT_PAGE_INTRO)
	assistant.SetPageType(page2, gtk.ASSISTANT_PAGE_CONTENT)
	assistant.SetPageType(page3, gtk.ASSISTANT_PAGE_CONTENT)
	assistant.SetPageType(page4, gtk.ASSISTANT_PAGE_CONFIRM)

	assistant.SetPageTitle(page1, "Welcome")
	assistant.SetPageTitle(page2, "Regions")
	assistant.SetPageTitle(page3, "Platforms")
	assistant.SetPageTitle(page4, "Finish")

	assistant.Connect("apply", func() {
		config.DidInitialSetup = true
		selectedRegions := uint8(0)
		if europeCheck.GetActive() {
			selectedRegions |= wiiudownloader.MCP_REGION_EUROPE
		}
		if usaCheck.GetActive() {
			selectedRegions |= wiiudownloader.MCP_REGION_USA
		}
		if japanCheck.GetActive() {
			selectedRegions |= wiiudownloader.MCP_REGION_JAPAN
		}
		config.SelectedRegion = selectedRegions
		config.DecryptContents = cemuCheck.GetActive()
		config.DeleteEncryptedContents = !wiiUCheck.GetActive()
		if err := config.Save(); err != nil {
			ShowErrorDialog(nil, fmt.Errorf("Failed to save config: %w", err))
			return
		}
		assistant.Hide()
		assistant.Emit("close", glib.TYPE_BOOLEAN, nil)
		assistant.SetDestroyWithParent(true)
	})

	initialSetupAssistantWindow := InitialSetupAssistantWindow{
		assistantWindow: assistant,
		config:          config,
		skipButton:      skipButton,
	}

	return &initialSetupAssistantWindow, nil
}

func (assistant *InitialSetupAssistantWindow) ShowAll() {
	assistant.assistantWindow.ShowAll()
}

func (assistant *InitialSetupAssistantWindow) Hide() {
	assistant.assistantWindow.Hide()
}

func (assistant *InitialSetupAssistantWindow) SetPostSetupCallback(cb func()) {
	assistant.assistantWindow.Connect("apply", cb)
	assistant.skipButton.Connect("clicked", cb)
}
