#include <GameList.h>
#include <cdecrypt/cdecrypt.h>
#include <utils.h>
#include <tmd.h>

#include <cstdlib>
#include <downloader.h>
#include <iostream>
#include <utility>

void GameList::updateTitles(TITLE_CATEGORY cat, MCPRegion reg) {
    treeModel = Gtk::ListStore::create(columns);
    treeView->set_model(treeModel);
    treeView->get_selection()->set_mode(Gtk::SELECTION_MULTIPLE);
    for (unsigned int i = 0; i < getTitleEntriesSize(cat); i++) {
        if (!(reg & infos[i].region))
            continue;
        char id[128];
        hex(infos[i].tid, 16, id);
        Gtk::TreeModel::Row row = *(treeModel->append());
        row[columns.index] = i;
        row[columns.toQueue] = queueMap.empty() ? false : queueMap.find(infos[i].tid) != queueMap.end();
        row[columns.name] = infos[i].name;
        row[columns.region] = Glib::ustring::format(getFormattedRegion((MCPRegion) infos[i].region));
        row[columns.kind] = Glib::ustring::format(getFormattedKind(infos[i].tid));
        row[columns.titleId] = Glib::ustring::format(id);
    }
}

GameList::GameList(Glib::RefPtr<Gtk::Application> app, const Glib::RefPtr<Gtk::Builder>& builder, const TitleEntry *infos) {
    this->app = std::move(app);
    this->builder = builder;
    this->infos = infos;

    builder->get_widget("gameListWindow", gameListWindow);
    gameListWindow->show();

    builder->get_widget("gamesButton", gamesButton);
    gamesButton->set_active();
    gamesButton->signal_button_press_event().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_button_selected), TITLE_CATEGORY_GAME));

    builder->get_widget("updatesButton", updatesButton);
    updatesButton->signal_button_press_event().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_button_selected), TITLE_CATEGORY_UPDATE));

    builder->get_widget("dlcsButton", dlcsButton);
    dlcsButton->signal_button_press_event().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_button_selected), TITLE_CATEGORY_DLC));

    builder->get_widget("demoButton", demosButton);
    demosButton->signal_button_press_event().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_button_selected), TITLE_CATEGORY_DEMO));

    builder->get_widget("allButton", allButton);
    allButton->signal_button_press_event().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_button_selected), TITLE_CATEGORY_ALL));

    builder->get_widget("addToQueueButton", addToQueueButton);
    addToQueueButton->signal_button_press_event().connect_notify(sigc::mem_fun(*this, &GameList::on_add_to_queue));

    builder->get_widget("downloadQueueButton", downloadQueueButton);
    downloadQueueButton->signal_button_press_event().connect_notify(sigc::mem_fun(*this, &GameList::on_download_queue));

    builder->get_widget("japanButton", japanButton);
    japanButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_region_selected), japanButton, MCP_REGION_JAPAN));

    builder->get_widget("usaButton", usaButton);
    usaButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_region_selected), usaButton, MCP_REGION_USA));

    builder->get_widget("europeButton", europeButton);
    europeButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_region_selected), europeButton, MCP_REGION_EUROPE));

    builder->get_widget("decryptContentsButton", decryptContentsButton);
    decryptContentsButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_decrypt_selected), decryptContentsButton));

    builder->get_widget("deleteEncryptedContentsButton", deleteEncryptedContentsButton);
    deleteEncryptedContentsButton->signal_toggled().connect_notify(sigc::bind(sigc::mem_fun(*this, &GameList::on_delete_encrypted_selected), deleteEncryptedContentsButton));
    deleteEncryptedContentsButton->set_sensitive(FALSE);

    builder->get_widget("searchBar", searchBar);
    builder->get_widget("searchEntry", searchEntry);
    searchEntry->signal_changed().connect(sigc::mem_fun(*this, &GameList::search_entry_changed));

    builder->get_widget("gameTree", treeView);
    treeView->signal_row_activated().connect(sigc::mem_fun(*this, &GameList::on_gamelist_row_activated));
    treeView->get_selection()->signal_changed().connect(sigc::mem_fun(*this, &GameList::on_selection_changed));

    Gtk::ImageMenuItem *decryptMenuButton = nullptr;
    builder->get_widget("decryptMenuButton", decryptMenuButton);
    decryptMenuButton->signal_activate().connect(sigc::mem_fun(*this, &GameList::on_decrypt_menu_click));

    Gtk::ImageMenuItem *generateFakeTIKButton = nullptr;
    builder->get_widget("generateFakeTIKButton", generateFakeTIKButton);
    generateFakeTIKButton->signal_activate().connect(sigc::mem_fun(*this, &GameList::on_generate_fake_tik_menu_click));

    updateTitles(currentCategory, selectedRegion);

    Gtk::CellRendererToggle *renderer = Gtk::manage(new Gtk::CellRendererToggle());
    int cols_count = treeView->append_column("Queue", *renderer);
    Gtk::TreeViewColumn *pColumn = treeView->get_column(cols_count - 1);

    pColumn->add_attribute(*renderer, "active", columns.toQueue);

    treeView->append_column("TitleID", columns.titleId);
    treeView->get_column(1)->set_sort_column(columns.titleId);

    treeView->append_column("Kind", columns.kind);

    treeView->append_column("Region", columns.region);

    treeView->append_column("Name", columns.name);
    treeView->get_column(4)->set_sort_column(columns.name);

    // Search for name
    treeView->set_search_column(5);

    // Sort by name by default
    treeModel->set_sort_column(GTK_TREE_SORTABLE_UNSORTED_SORT_COLUMN_ID, Gtk::SortType::SORT_ASCENDING);
    treeModel->set_sort_column(5, Gtk::SortType::SORT_ASCENDING);

    treeView->set_search_equal_func(sigc::mem_fun(*this, &GameList::on_search_equal));
}

GameList::~GameList() {
    free(cancelQueue);
}

void GameList::search_entry_changed() {
    m_refTreeModelFilter = Gtk::TreeModelFilter::create(treeModel);
    m_refTreeModelFilter->set_visible_func(
            [this](const Gtk::TreeModel::const_iterator &iter) -> bool {
                if (!iter)
                    return true;

                const Gtk::TreeModel::Row& row = *iter;
                Glib::ustring name = row[columns.name];
                Glib::ustring key = searchEntry->get_text();
                std::string string_name(name.lowercase());
                std::string string_key(key.lowercase());
                if (string_name.find(string_key) != Glib::ustring::npos) {
                    return true;
                }

                Glib::ustring titleId = row[columns.titleId];
                if (strcmp(titleId.c_str(), key.c_str()) == 0) {
                    return true;
                }

                return false;
            });
    treeView->set_model(m_refTreeModelFilter);
}

void GameList::on_decrypt_selected(Gtk::ToggleButton *button) {
    // decryptContents = !decryptContents; Bug Fix Attempt
    if(button->get_active())
    {
        decryptContents = true;
        deleteEncryptedContentsButton->set_sensitive(TRUE);
    }
    else {
        decryptContents = false;
        deleteEncryptedContentsButton->set_sensitive(FALSE);
        deleteEncryptedContentsButton->set_active(FALSE);
        deleteEncryptedContents = false;
    }
}

void GameList::on_delete_encrypted_selected(Gtk::ToggleButton *button) {
    // deleteEncryptedContents = !deleteEncryptedContents; Bug Fix Attempt 2
    // I think this way is just better because it ensures consistency no matter what the variable
    // was previously set to.
    if(button->get_active()) {
        deleteEncryptedContents = true;
    } else {
        deleteEncryptedContents = false;
    }
}

void GameList::on_download_queue(GdkEventButton *ev) {
    if (queueMap.empty())
        return;
    gameListWindow->set_sensitive(false);
    *cancelQueue = false;
    for (auto queuedItem : queueMap) {
        char tid[128];
        sprintf(tid, "%016llx", queuedItem.first);
        downloadTitle(tid, queuedItem.second, decryptContents, cancelQueue, deleteEncryptedContents, true);
    }
    Glib::RefPtr<Gio::Notification> notification = Gio::Notification::create("WiiUDownloader");
    notification->set_body("Queue download(s) finished");
    this->app->send_notification(notification);
    *cancelQueue = false;
    queueMap.clear();
    updateTitles(currentCategory, selectedRegion);
    gameListWindow->set_sensitive(true);
}

bool GameList::is_selection_in_queue() {
    //Gtk::TreeModel::Row row = *selection->get_selected();
    std::vector<Gtk::TreeModel::Path> pathlist = treeView->get_selection()->get_selected_rows();
    for (auto iter = pathlist.begin(); iter != pathlist.end(); iter++) {
        Gtk::TreeModel::Row row = *(treeView->get_model()->get_iter(*iter));
        if (!row) continue;
        if (row[columns.toQueue] == false) {
            return false;
        }
    }
    return true;
}

void GameList::on_selection_changed() {
    if (treeView->get_selection()->get_selected_rows().size() == 0) {
        // Change the label to "add" when nothing is selected because it's like the default option.
        addToQueueButton->set_label("Add to queue");
        addToQueueButton->set_sensitive(false);
        return;
    }
    addToQueueButton->set_sensitive(true);
    if (is_selection_in_queue()) {
        addToQueueButton->set_label("Remove from queue");
    } else {
        addToQueueButton->set_label("Add to queue");
    }
}

void GameList::on_add_to_queue(GdkEventButton *ev) {
    //Gtk::TreeModel::Row row = *selection->get_selected();
    std::vector<Gtk::TreeModel::Path> pathlist = treeView->get_selection()->get_selected_rows();
    bool updateAsked = false;
    bool updateSelected;
    bool addToQueue = !is_selection_in_queue();
    for (auto iter = pathlist.begin(); iter != pathlist.end(); iter++) {
        Gtk::TreeModel::Row row = *(treeView->get_model()->get_iter(*iter));
        // If (!row) or if row is already in the correct place (queue or not), then skip.
        if (!row || addToQueue == row[columns.toQueue]) continue;
        row[columns.toQueue] = addToQueue;
        if (addToQueue) {
            queueMap.emplace(std::pair(infos[row[columns.index]].tid, infos[row[columns.index]].name));
            uint64_t updateTID = 0;
            if (getUpdateFromBaseGame(infos[row[columns.index]].tid, &updateTID)) {
                if (!updateAsked) {
                    updateAsked = true;
                    updateSelected = ask("Update(s) detected.\nDo you want to add the update(s) to the queue too?");
                }
                if (updateSelected) {
                    queueMap.emplace(std::pair(updateTID, infos[row[columns.index]].name));
                }
            }
            addToQueueButton->set_label("Remove from queue");
        } else {
            queueMap.erase(infos[row[columns.index]].tid);
            uint64_t updateTID = 0;
            if (getUpdateFromBaseGame(infos[row[columns.index]].tid, &updateTID)) {
                bool updateInQueue = queueMap.empty() ? false : queueMap.find(updateTID) != queueMap.end();
                if (updateInQueue) {
                    if (!updateAsked) {
                        updateAsked = true;
                        updateSelected = ask("Update detected.\nDo you want to remove the update from the queue too?");
                    }
                    if (updateSelected) {
                        queueMap.erase(updateTID);
                    }
                }
            }
            addToQueueButton->set_label("Add to queue");
        }
    }
}

void GameList::on_button_selected(GdkEventButton *ev, TITLE_CATEGORY cat) {
    currentCategory = cat;
    infos = getTitleEntries(currentCategory);
    updateTitles(currentCategory, selectedRegion);
    search_entry_changed();
}

void GameList::on_region_selected(Gtk::ToggleButton *button, MCPRegion reg) {
    if (button->get_active())
        selectedRegion = (MCPRegion) (selectedRegion | reg);
    else
        selectedRegion = (MCPRegion) (selectedRegion & ~reg);
    updateTitles(currentCategory, selectedRegion);
    search_entry_changed();
}

void GameList::on_gamelist_row_activated(const Gtk::TreePath &treePath, Gtk::TreeViewColumn *const &column) {
    Glib::RefPtr<Gtk::TreeSelection> selection = treeView->get_selection();
    Gtk::TreeModel::Row row = *selection->get_selected();
    if (row) {
        gameListWindow->set_sensitive(false);
        char selectedTID[128];
        sprintf(selectedTID, "%016llx", infos[row[columns.index]].tid);
        *cancelQueue = false;
        downloadTitle(selectedTID, infos[row[columns.index]].name, decryptContents, cancelQueue, deleteEncryptedContents, true);
        Glib::RefPtr<Gio::Notification> notification = Gio::Notification::create("WiiUDownloader");
        notification->set_body("Download finished");
        this->app->send_notification(notification);
        *cancelQueue = false;
        gameListWindow->set_sensitive(true);
    }
}

bool GameList::on_search_equal(const Glib::RefPtr<Gtk::TreeModel> &model, int column, const Glib::ustring &key, const Gtk::TreeModel::iterator &iter) const {
    const Gtk::TreeModel::Row& row = *iter;
    if (row) {
        Glib::ustring name = row[columns.name];
        std::string string_name(name.lowercase());
        std::string string_key(key.lowercase());
        if (string_name.find(string_key) != std::string::npos) {
            return false;
        }

        Glib::ustring titleId = row[columns.titleId];
        if (strcmp(titleId.c_str(), key.c_str()) == 0) {
            return false;
        }
    }
    return true;
}

void GameList::on_decrypt_menu_click() {
    char *selectedPath = show_folder_select_dialog();
    if (selectedPath == nullptr)
        return;

    char *argv[2] = {(char *) "WiiUDownloader", selectedPath};
    if (cdecrypt(2, argv, true) != 0)
        showError("Error: There was a problem decrypting the files.\nThe path specified for the download might be too long.\nPlease try downloading the files to a shorter path and try again.");
}

void GameList::on_generate_fake_tik_menu_click() {
    char *selectedPath = show_folder_select_dialog();
    if (selectedPath == nullptr)
        return;
    std::string path(selectedPath);
    FILE *tmd = fopen((path + "/title.tmd").c_str(), "rb");
    if(tmd == nullptr) {
        showError("Error 1 while creating ticket!\nTicket can't be opened or not found");
        return;
    }
    fseek(tmd, 0L, SEEK_END);
    size_t fSize = ftell(tmd);
    fseek(tmd, 0L, SEEK_SET);
    uint8_t buffer[2048];
    fread(buffer, fSize, 1, tmd);
    TMD *tmdData = (TMD *) buffer;
    uint16_t titleVersion = bswap_16(tmdData->title_version);
    char titleID[17];
    sprintf(titleID, "%016llx", bswap_64(tmdData->tid));
    char titleKey[128];
    generateKey(titleID, titleKey);
    if(!generateTicket((path + "/title.tik").c_str(), bswap_64(tmdData->tid), titleKey, titleVersion))
        showError("Error 2 while creating ticket!\nCouldn't write ticket");
}