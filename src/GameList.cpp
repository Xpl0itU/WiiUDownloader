#include <GameList.h>

#include <cstdlib>
#include <titleInfo.h>
#include <downloader.h>
#include <iostream>

TITLE_CATEGORY currentCategory = TITLE_CATEGORY_ALL;

GameList::GameList(Glib::RefPtr<Gtk::Builder> builder, const TitleEntry *infos)
{
    this->builder = builder;
    this->infos = infos;

    builder->get_widget("gameListWindow", gameListWindow);
    gameListWindow->show();

    builder->get_widget("gamesButton", gamesButton);
    gamesButton->set_active();
    gamesButton->signal_button_press_event().connect_notify(sigc::mem_fun(*this, &GameList::on_button_selected));

    builder->get_widget("updatesButton", updatesButton);

    builder->get_widget("dlcsButton", dlcsButton);

    builder->get_widget("demoButton", demosButton);

    builder->get_widget("allButton", allButton);

    builder->get_widget("gameTree", treeView);
    treeView->signal_row_activated().connect(sigc::mem_fun(*this, &GameList::on_gamelist_row_activated));
    
    Glib::RefPtr<Gtk::ListStore> treeModel = Gtk::ListStore::create(columns);
    treeView->set_model(treeModel);

    for (unsigned int i = 0; i < getTitleEntriesSize(currentCategory); i++)
    {
        char id[128];
        hex(infos[i].tid, 16, id);
        Gtk::TreeModel::Row row = *(treeModel->append());

        row[columns.index] = i;
        row[columns.name] = infos[i].name;
        row[columns.region] = Glib::ustring::format(getFormattedRegion(infos[i].region));
        row[columns.kind] = Glib::ustring::format(getFormattedKind(infos[i].tid));
        row[columns.titleId] = Glib::ustring::format(id);
    }

    treeView->append_column("TitleID", columns.titleId);
    treeView->get_column(0);
    treeView->get_column(0);

    treeView->append_column("Kind", columns.kind);
    treeView->get_column(1);
    treeView->get_column(1);

    treeView->append_column("Region", columns.region);
    treeView->get_column(2);
    treeView->get_column(2);

    treeView->append_column("Name", columns.name);
    treeView->get_column(3);
    treeView->get_column(3);

    // Search for name
    treeView->set_search_column(4);

    // Sort by name by default
    treeModel->set_sort_column(GTK_TREE_SORTABLE_UNSORTED_SORT_COLUMN_ID, Gtk::SortType::SORT_ASCENDING);
    treeModel->set_sort_column(4, Gtk::SortType::SORT_ASCENDING);

    treeView->set_search_equal_func(sigc::mem_fun(*this, &GameList::on_search_equal));
}

GameList::~GameList()
{
    
}

void GameList::on_button_selected(GdkEventButton* ev) {
    fprintf(stderr, "button changed");
    return;
}

void GameList::on_gamelist_row_activated(const Gtk::TreePath& treePath, Gtk::TreeViewColumn* const& column)
{
    Glib::RefPtr<Gtk::TreeSelection> selection = treeView->get_selection();
    Gtk::TreeModel::Row row = *selection->get_selected();

    gameListWindow->set_sensitive(false);
    char selectedTID[128];
    sprintf(selectedTID, "%016llx", infos[row[columns.index]].tid);
    downloadTitle(selectedTID);
    gameListWindow->set_sensitive(true);
}

bool GameList::on_search_equal(const Glib::RefPtr<Gtk::TreeModel>& model, int column, const Glib::ustring& key, const Gtk::TreeModel::iterator& iter)
{
    Gtk::TreeModel::Row row = *iter;

    Glib::ustring name = row[columns.name];
    std::string string_name(name.lowercase());
    std::string string_key(key.lowercase());
    if (string_name.find(string_key) != std::string::npos)
    {
        return false;
    }

    Glib::ustring titleId = row[columns.titleId];
    if (strcmp(titleId.c_str(), key.c_str()) == 0)
    {
        return false;
    }

    return true;
}