#include <utils.h>
#include <GameList.h>

#include <cstdlib>
#include <downloader.h>
#include <iostream>

void GameList::updateTitles(TITLE_CATEGORY cat, MCPRegion reg) {
    treeModel = Gtk::ListStore::create(columns);
    treeView->set_model(treeModel);
    for (unsigned int i = 0; i < getTitleEntriesSize(cat); i++) {
        if (!(reg & infos[i].region))
            continue;
        char id[128];
        hex(infos[i].tid, 16, id);
        Gtk::TreeModel::Row row = *(treeModel->append());
        row[columns.index] = i;
        row[columns.toQueue] = queueVector.empty() ? false : std::binary_search(queueVector.begin(), queueVector.end(), infos[i].tid);
        row[columns.name] = infos[i].name;
        row[columns.region] = Glib::ustring::format(getFormattedRegion((MCPRegion) infos[i].region));
        row[columns.kind] = Glib::ustring::format(getFormattedKind(infos[i].tid));
        row[columns.titleId] = Glib::ustring::format(id);
    }
}

GameList::GameList(Glib::RefPtr<Gtk::Builder> builder, const TitleEntry *infos) {
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
    decryptContentsButton->set_active(TRUE);

    builder->get_widget("searchBar", searchBar);
    builder->get_widget("searchEntry", searchEntry);
    searchEntry->signal_changed().connect(sigc::mem_fun(*this, &GameList::search_entry_changed));

    builder->get_widget("gameTree", treeView);
    treeView->signal_row_activated().connect(sigc::mem_fun(*this, &GameList::on_gamelist_row_activated));
    treeView->get_selection()->signal_changed().connect(sigc::mem_fun(*this, &GameList::on_selection_changed));

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
}

bool ask(Glib::ustring question){
	Glib::ustring msg = Glib::ustring(question);
	Gtk::MessageDialog dlg(msg, true, Gtk::MESSAGE_QUESTION, Gtk::BUTTONS_YES_NO, true);
	return dlg.run() == Gtk::RESPONSE_YES;
}

void GameList::search_entry_changed() {
    m_refTreeModelFilter = Gtk::TreeModelFilter::create(treeModel);
    m_refTreeModelFilter->set_visible_func(
            [this](const Gtk::TreeModel::const_iterator &iter) -> bool {
                if (!iter)
                    return true;

                Gtk::TreeModel::Row row = *iter;
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
    decryptContents = !decryptContents;
    return;
}

void GameList::on_download_queue(GdkEventButton *ev) {
    if (queueVector.empty())
        return;
    gameListWindow->set_sensitive(false);
    for (auto queuedItem : queueVector) {
        char tid[128];
        sprintf(tid, "%016llx", queuedItem);
        downloadTitle(tid, decryptContents);
    }
    queueVector.clear();
    gameListWindow->set_sensitive(true);
}

void GameList::on_selection_changed() {
    Glib::RefPtr<Gtk::TreeSelection> selection = treeView->get_selection();
    Gtk::TreeModel::Row row = *selection->get_selected();
    if (row) {
        if (row[columns.toQueue] == true) {
            addToQueueButton->set_label("Remove from queue");
        } else {
            addToQueueButton->set_label("Add to queue");
        }
    }
}

void GameList::on_add_to_queue(GdkEventButton *ev) {
    Glib::RefPtr<Gtk::TreeSelection> selection = treeView->get_selection();
    Gtk::TreeModel::Row row = *selection->get_selected();
    if (row) {
        row[columns.toQueue] = !row[columns.toQueue];
        if (row[columns.toQueue]) {
            queueVector.push_back(infos[row[columns.index]].tid);
            uint64_t updateTID = 0;
            if(getUpdateFromBaseGame(infos[row[columns.index]].tid, &updateTID))
                if(ask("Update detected.\nDo you want to add the update to the queue too?"))
                    queueVector.push_back(updateTID);
            addToQueueButton->set_label("Remove from queue");
        } else {
            queueVector.erase(std::remove(queueVector.begin(), queueVector.end(), infos[row[columns.index]].tid), queueVector.end());
            uint64_t updateTID = 0;
            if(getUpdateFromBaseGame(infos[row[columns.index]].tid, &updateTID)) {
                bool updateInQueue = queueVector.empty() ? false : std::binary_search(queueVector.begin(), queueVector.end(), updateTID);
                if(updateInQueue)
                    if(ask("Update detected.\nDo you want to remove the update from the queue too?"))
                        queueVector.erase(std::remove(queueVector.begin(), queueVector.end(), updateTID), queueVector.end());
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
    return;
}

void GameList::on_region_selected(Gtk::ToggleButton *button, MCPRegion reg) {
    if (button->get_active())
        selectedRegion = (MCPRegion) (selectedRegion | reg);
    else
        selectedRegion = (MCPRegion) (selectedRegion & ~reg);
    updateTitles(currentCategory, selectedRegion);
    search_entry_changed();
    return;
}

void GameList::on_gamelist_row_activated(const Gtk::TreePath &treePath, Gtk::TreeViewColumn *const &column) {
    Glib::RefPtr<Gtk::TreeSelection> selection = treeView->get_selection();
    Gtk::TreeModel::Row row = *selection->get_selected();
    if (row) {
        gameListWindow->set_sensitive(false);
        char selectedTID[128];
        sprintf(selectedTID, "%016llx", infos[row[columns.index]].tid);
        downloadTitle(selectedTID, decryptContents);
        gameListWindow->set_sensitive(true);
    }
}

bool GameList::on_search_equal(const Glib::RefPtr<Gtk::TreeModel> &model, int column, const Glib::ustring &key, const Gtk::TreeModel::iterator &iter) {
    Gtk::TreeModel::Row row = *iter;
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