#include <GameList.h>

#include <downloader.h>
#include <iostream>

GameList::GameList(Glib::RefPtr<Gtk::Builder> builder, const TitleEntry *infos)
{
    this->builder = builder;
    this->infos = infos;

    builder->get_widget("gameListWindow", gameListWindow);
    gameListWindow->show();

    builder->get_widget("gameTree", treeView);
    treeView->signal_row_activated().connect(sigc::mem_fun(*this, &GameList::on_gamelist_row_activated));
    
    Glib::RefPtr<Gtk::ListStore> treeModel = Gtk::ListStore::create(columns);
    treeView->set_model(treeModel);

    for (unsigned int i = 0; i < getTitleEntriesSize(TITLE_CATEGORY_ALL); i++)
    {
        char id[128];
        hex(infos[i].tid, 16, id);
        Gtk::TreeModel::Row row = *(treeModel->append());

        row[columns.index] = i;
        row[columns.name] = infos[i].name;
        row[columns.titleId] = Glib::ustring::format(id);
    }

    treeView->append_column("TitleID", columns.titleId);
    treeView->get_column(1);
    treeView->get_column(1);

    treeView->append_column("Name", columns.name);
    treeView->get_column(2);
    treeView->get_column(2);
}

GameList::~GameList()
{
    
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