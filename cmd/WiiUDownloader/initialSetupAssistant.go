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
	assistant.SetDefaultSize(500, 400)

	page1, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page1.SetBorderWidth(10)
	assistant.AppendPage(page1)

	page1Label, err := gtk.LabelNew("Welcome to WiiUDownloader! This assistant will help you set up the program.\nNote: This is a one-time setup, you can always change these settings from the program interface itself.")
	if err != nil {
		return nil, err
	}
	page1.PackStart(page1Label, true, true, 0)

	// Skip button
	skipButton, err := gtk.ButtonNewWithLabel("Skip")
	if err != nil {
		return nil, err
	}
	assistant.AddActionWidget(skipButton)
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
	page2.SetBorderWidth(10)
	assistant.AppendPage(page2)

	page2Label, err := gtk.LabelNew("Please select your region(s):")
	if err != nil {
		return nil, err
	}
	page2.PackStart(page2Label, true, true, 0)

	regionBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	page2.PackStart(regionBox, true, true, 0)

	selectedRegionCheckboxes := uint8(0)

	europeCheck, err := gtk.CheckButtonNewWithLabel("Europe")
	if err != nil {
		return nil, err
	}
	europeCheck.Connect("toggled", func() {
		if europeCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
	regionBox.PackStart(europeCheck, true, true, 0)

	usaCheck, err := gtk.CheckButtonNewWithLabel("USA")
	if err != nil {
		return nil, err
	}
	usaCheck.Connect("toggled", func() {
		if usaCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
	regionBox.PackStart(usaCheck, true, true, 0)

	japanCheck, err := gtk.CheckButtonNewWithLabel("Japan")
	if err != nil {
		return nil, err
	}
	japanCheck.Connect("toggled", func() {
		if japanCheck.GetActive() {
			selectedRegionCheckboxes++
		} else {
			selectedRegionCheckboxes--
		}
		assistant.SetPageComplete(page2, selectedRegionCheckboxes > 0)
	})
	regionBox.PackStart(japanCheck, true, true, 0)

	page3, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	page3.SetBorderWidth(10)
	assistant.AppendPage(page3)

	page3Label, err := gtk.LabelNew("Please select your target platform(s):")
	if err != nil {
		return nil, err
	}
	page3.PackStart(page3Label, true, true, 0)

	cemuCheck, err := gtk.CheckButtonNewWithLabel("CEMU")
	if err != nil {
		return nil, err
	}
	cemuCheck.SetActive(true)

	wiiUCheck, err := gtk.CheckButtonNewWithLabel("Wii U")
	if err != nil {
		return nil, err
	}
	wiiUCheck.SetActive(true)

	platformBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err != nil {
		return nil, err
	}
	page3.PackStart(platformBox, true, true, 0)
	platformBox.PackStart(cemuCheck, true, true, 0)
	platformBox.PackStart(wiiUCheck, true, true, 0)

	page4, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0) // Finish page
	if err != nil {
		return nil, err
	}
	page4.SetBorderWidth(10)
	assistant.AppendPage(page4)

	page4Label, err := gtk.LabelNew("You have successfully set up WiiUDownloader!\nYou can always change these settings from the program interface itself.\nClick Apply to finish.")
	if err != nil {
		return nil, err
	}
	page4.PackStart(page4Label, true, true, 0)

	assistant.SetPageComplete(page1, true)
	assistant.SetPageComplete(page2, false)
	assistant.SetPageComplete(page3, true) // TODO: Check if at least one platform is selected
	assistant.SetPageComplete(page4, true)

	assistant.SetPageType(page1, gtk.ASSISTANT_PAGE_INTRO)
	assistant.SetPageType(page2, gtk.ASSISTANT_PAGE_PROGRESS)
	assistant.SetPageType(page3, gtk.ASSISTANT_PAGE_PROGRESS)
	assistant.SetPageType(page4, gtk.ASSISTANT_PAGE_CONFIRM)

	assistant.SetPageTitle(page1, "Welcome")
	assistant.SetPageTitle(page2, "Region")
	assistant.SetPageTitle(page3, "Platform")
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
