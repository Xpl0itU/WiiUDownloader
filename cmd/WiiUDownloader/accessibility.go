package main

import (
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func setTooltip(widget gtk.Widgetter, text string) {
	if widget == nil {
		return
	}
	gtk.BaseWidget(widget).SetTooltipText(text)
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
	setTooltip(button, composeAccessibleText(button.Label(), description, " - "))
	return nil
}

// SetupEntryAccessibility accepts any entry widget, including GtkSearchEntry.
func SetupEntryAccessibility(entry gtk.Widgetter, label, description string) error {
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

func SetupToggleButtonAccessibility(button *gtk.ToggleButton, description string) error {
	if button == nil {
		return nil
	}
	setTooltip(button, composeAccessibleText(button.Label(), description, ". "))
	return nil
}

func SetupCheckButtonAccessibility(checkButton *gtk.CheckButton, description string) error {
	if checkButton == nil {
		return nil
	}
	setTooltip(checkButton, composeAccessibleText(checkButton.Label(), description, ". "))
	return nil
}

// SetupListViewAccessibility makes a list widget keyboard-focusable.
func SetupListViewAccessibility(list gtk.Widgetter) error {
	if list == nil {
		return nil
	}
	gtk.BaseWidget(list).SetCanFocus(true)
	return nil
}

func SetupWindowAccessibility(window *gtk.Window, _ string) error {
	if window == nil {
		return nil
	}
	return nil
}

func isKeyboardActivationKey(keyVal uint) bool {
	return keyVal == gdk.KEY_space || keyVal == gdk.KEY_Return || keyVal == gdk.KEY_KP_Enter
}
