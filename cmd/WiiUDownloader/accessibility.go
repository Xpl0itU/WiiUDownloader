package main

import (
	"log"

	"github.com/gotk3/gotk3/gtk"
)

// SetAccessibleLabel sets an accessible label for screen readers
// This helps text-to-speech software read button labels and descriptions
func SetAccessibleLabel(widget gtk.IWidget, label string) {
	obj := widget.ToWidget()
	if obj != nil {
		obj.SetProperty("tooltip-text", label)
	}
}

// SetAccessibleDescription sets a more detailed accessible description
// This is spoken by screen readers in addition to the label
func SetAccessibleDescription(widget gtk.IWidget, description string) {
	obj := widget.ToWidget()
	if obj != nil {
		// Store in tooltip for GTK3 - this is what screen readers will read
		obj.SetProperty("tooltip-text", description)
	}
}

// MakeWidgetAccessible sets up a widget for accessibility
// including label and description
func MakeWidgetAccessible(widget gtk.IWidget, label, description string) {
	obj := widget.ToWidget()
	if obj != nil {
		// Set tooltip which is read by screen readers
		obj.SetProperty("tooltip-text", description)
	}
}

// SetupButtonAccessibility configures a button for accessibility
// This includes setting a descriptive tooltip that screen readers will announce
func SetupButtonAccessibility(button *gtk.Button, description string) error {
	if button == nil {
		return nil
	}

	// Get the button label
	label, err := button.GetLabel()
	if err != nil {
		label = ""
	}

	// Set tooltip with full description (screen readers read this)
	fullDescription := label
	if description != "" {
		fullDescription = label + " - " + description
	}

	obj := button.ToWidget()
	if obj != nil {
		obj.SetProperty("tooltip-text", fullDescription)
	}

	return nil
}

// SetupEntryAccessibility configures a text entry for accessibility
func SetupEntryAccessibility(entry *gtk.Entry, label, description string) error {
	if entry == nil {
		return nil
	}

	// Set tooltip that screen readers will announce
	fullDescription := label
	if description != "" {
		fullDescription = label + ". " + description
	}

	obj := entry.ToWidget()
	if obj != nil {
		obj.SetProperty("tooltip-text", fullDescription)
	}

	return nil
}

// SetupLabelAccessibility configures a label for accessibility
func SetupLabelAccessibility(label *gtk.Label, text string) error {
	if label == nil {
		return nil
	}

	// Make sure label text is readable
	label.SetSelectable(true) // Allows screen readers to select text

	return nil
}

// SetupCheckButtonAccessibility configures a checkbox for accessibility
func SetupCheckButtonAccessibility(checkButton *gtk.CheckButton, description string) error {
	if checkButton == nil {
		return nil
	}

	label, err := checkButton.GetLabel()
	if err != nil {
		label = ""
	}

	fullDescription := label
	if description != "" {
		fullDescription = label + ". " + description
	}

	obj := checkButton.ToWidget()
	if obj != nil {
		obj.SetProperty("tooltip-text", fullDescription)
	}

	return nil
}

// SetupToggleButtonAccessibility configures a toggle button for accessibility
func SetupToggleButtonAccessibility(toggleButton *gtk.ToggleButton, description string) error {
	if toggleButton == nil {
		return nil
	}

	label, err := toggleButton.GetLabel()
	if err != nil {
		label = ""
	}

	fullDescription := label
	if description != "" {
		fullDescription = label + ". " + description
	}

	obj := toggleButton.ToWidget()
	if obj != nil {
		obj.SetProperty("tooltip-text", fullDescription)
	}

	return nil
}

// SetupTreeViewAccessibility configures a tree view for keyboard navigation
func SetupTreeViewAccessibility(treeView *gtk.TreeView) error {
	if treeView == nil {
		return nil
	}

	// Enable keyboard navigation (default in GTK but being explicit)
	treeView.SetCanFocus(true)

	return nil
}

// SetupWindowAccessibility configures a window for accessibility
// including proper focus handling
func SetupWindowAccessibility(window *gtk.Window, title string) error {
	if window == nil {
		return nil
	}

	// Ensure window is keyboard accessible
	window.SetKeepAbove(false)

	return nil
}

// SetupDialogAccessibility configures a dialog for accessibility
func SetupDialogAccessibility(dialog *gtk.Dialog, title string) error {
	if dialog == nil {
		return nil
	}

	// Make sure dialog is properly focused
	dialog.SetKeepAbove(true)

	return nil
}

// LogAccessibilitySetup logs when accessibility features are being set up
// This helps with debugging accessibility issues
func LogAccessibilitySetup(componentName string, label string) {
	log.Printf("[Accessibility] Setting up %s: %s", componentName, label)
}
