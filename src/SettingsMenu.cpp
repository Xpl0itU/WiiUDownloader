#include <SettingsMenu.h>

#include <downloader.h>
#include <settings.h>
#include <utils.h>

SettingsMenu::SettingsMenu(const Glib::RefPtr<Gtk::Builder> &builder) {
    this->builder = builder;

    builder->get_widget("settingsDialog", settingsDialog);
    settingsDialog->set_title("WiiUDownloader Settings");
    settingsDialog->show();

    builder->get_widget("downloadDirectoryEntry", downloadDirectoryEntry);
    downloadDirectoryEntry->set_text(getSelectedDir());

    builder->get_widget("browseDownloadDirButton", browseDownloadDirButton);
    browseDownloadDirButton->signal_pressed().connect(sigc::mem_fun(*this, &SettingsMenu::on_browse_download_dir));

    builder->get_widget("hideWiiVCWarningCheckButton", hideWiiVCWarningCheckButton);
    hideWiiVCWarningCheckButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &SettingsMenu::on_select_wiivc_hide_change), hideWiiVCWarningCheckButton));
    hideWiiVCWarningCheckButton->set_active(getHideWiiVCWarning());

    builder->get_widget("acceptSettingsButton", acceptSettingsButton);
    acceptSettingsButton->signal_button_press_event().connect_notify(sigc::mem_fun(*this, &SettingsMenu::on_accept_settings));

    builder->get_widget("cancelSettingsButton", cancelSettingsButton);
    cancelSettingsButton->signal_button_press_event().connect_notify(sigc::mem_fun(*this, &SettingsMenu::on_cancel_settings));
}

SettingsMenu::~SettingsMenu() = default;

void SettingsMenu::on_browse_download_dir() {
    char *selectedDir = show_folder_select_dialog();
    if(selectedDir != nullptr) {
        this->downloadDirectoryEntry->set_text(selectedDir);
        free(selectedDir);
    }
}

void SettingsMenu::on_select_wiivc_hide_change(Gtk::CheckButton *button) {
    if(button->get_active()) {
        setHideWiiVCWarning(true);
    } else {
        setHideWiiVCWarning(false);
    }
}

void SettingsMenu::on_accept_settings(GdkEventButton *ev) {
    saveSettings(downloadDirectoryEntry->get_text().c_str(), getHideWiiVCWarning());
    loadSettings();
    settingsDialog->hide();
}

void SettingsMenu::on_cancel_settings(GdkEventButton *ev) {
    settingsDialog->hide();
}