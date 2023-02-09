#pragma once

#include <gtkmm.h>

class SettingsMenu {
public:
    SettingsMenu(const Glib::RefPtr<Gtk::Builder>& builder);
    ~SettingsMenu();

    Gtk::Dialog *getWindow() { return settingsDialog; }

    void on_browse_download_dir(GdkEventButton *ev);
    void on_select_wiivc_hide_change(Gtk::CheckButton *button);
    void on_accept_settings(GdkEventButton *ev);
    void on_cancel_settings(GdkEventButton *ev);

private:
    Gtk::Dialog *settingsDialog = nullptr;
    Glib::RefPtr<Gtk::Builder> builder;

    Gtk::Entry *downloadDirectoryEntry = nullptr;
    Gtk::Button *browseDownloadDirButton = nullptr;
    Gtk::CheckButton *hideWiiVCWarningCheckButton = nullptr;

    Gtk::Button *acceptSettingsButton = nullptr;
    Gtk::Button *cancelSettingsButton = nullptr;

    Glib::RefPtr<Gtk::EntryBuffer> downloadDirectory;
    bool hideWiiVCWarning = false;
};