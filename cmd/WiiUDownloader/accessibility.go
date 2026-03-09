package main

import (
	"strings"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

func setTooltip(widget gtk.IWidget, text string) {
	if widget == nil {
		return
	}
	obj := widget.ToWidget()
	if obj == nil {
		return
	}
	obj.SetProperty("tooltip-text", text)
}

func composeAccessibleText(label, description, separator string) string {
	if description == "" {
		return label
	}
	if label == "" {
		return description
	}
	return strings.TrimSpace(label + separator + description)
}

func SetupButtonAccessibility(button *gtk.Button, description string) error {
	if button == nil {
		return nil
	}
	label, err := button.GetLabel()
	if err != nil {
		label = ""
	}
	setTooltip(button, composeAccessibleText(label, description, " - "))
	return nil
}

func SetupEntryAccessibility(entry *gtk.Entry, label, description string) error {
	if entry == nil {
		return nil
	}
	setTooltip(entry, composeAccessibleText(label, description, ". "))
	return nil
}

func SetupLabelAccessibility(label *gtk.Label, _ string) error {
	if label == nil {
		return nil
	}
	label.SetSelectable(true)
	return nil
}

func SetupCheckButtonAccessibility(checkButton *gtk.CheckButton, description string) error {
	if checkButton == nil {
		return nil
	}
	label, err := checkButton.GetLabel()
	if err != nil {
		label = ""
	}
	setTooltip(checkButton, composeAccessibleText(label, description, ". "))
	return nil
}

func SetupToggleButtonAccessibility(toggleButton *gtk.ToggleButton, description string) error {
	if toggleButton == nil {
		return nil
	}
	label, err := toggleButton.GetLabel()
	if err != nil {
		label = ""
	}
	setTooltip(toggleButton, composeAccessibleText(label, description, ". "))
	return nil
}

func SetupTreeViewAccessibility(treeView *gtk.TreeView) error {
	if treeView == nil {
		return nil
	}
	treeView.SetCanFocus(true)
	return nil
}

func SetupWindowAccessibility(window *gtk.Window, _ string) error {
	if window == nil {
		return nil
	}
	return nil
}

func SetupDialogAccessibility(dialog *gtk.Dialog, _ string) error {
	if dialog == nil {
		return nil
	}
	return nil
}

func isKeyboardActivationKey(keyVal uint) bool {
	return keyVal == gdk.KEY_space || keyVal == gdk.KEY_Return || keyVal == gdk.KEY_KP_Enter
}
