package main

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// newIconLabelButton builds a button whose label sits next to a symbolic icon;
// libadwaita lays the pair out and keeps them in sync with the button state.
func newIconLabelButton(iconName, label string) *gtk.Button {
	content := adw.NewButtonContent()
	content.SetIconName(iconName)
	content.SetLabel(label)
	button := gtk.NewButton()
	button.SetChild(content)
	return button
}

// newIconButton builds a flat, icon-only button whose purpose lives in the
// tooltip. Used where a labelled button would not fit the available width.
func newIconButton(iconName, tooltip string) *gtk.Button {
	button := gtk.NewButtonFromIconName(iconName)
	button.AddCSSClass("flat")
	SetupButtonAccessibility(button, tooltip)
	return button
}

// rowKey reads the "string" property of a GtkStringObject row, which the list
// stack uses as the row key (a title ID in hex).
func rowKey(obj *coreglib.Object) string {
	if obj == nil {
		return ""
	}
	key, _ := obj.ObjectProperty("string").(string)
	return key
}

// listItem re-wraps the GtkListItem gotk4 narrows to *coreglib.Object.
func listItem(obj *coreglib.Object) *gtk.ListItem {
	if obj == nil {
		return nil
	}
	return &gtk.ListItem{Object: obj}
}

// newRowStore creates the GtkStringList every table in this app is built on.
func newRowStore(keys []string) *gtk.StringList {
	return gtk.NewStringList(keys)
}

// listItemKey returns the bound row key, or "" during setup.
func listItemKey(item *gtk.ListItem) string {
	if item == nil {
		return ""
	}
	return rowKey(item.Item())
}
