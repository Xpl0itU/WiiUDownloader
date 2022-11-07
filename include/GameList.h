#pragma once

#include <gtkmm.h>
#include <string>
#include <vector>

#include <keygen.h>
#include <gtitles.h>

class GameList
{
public:
    class ModelColumns : public Gtk::TreeModel::ColumnRecord
    {
    public:
        ModelColumns()
        {
            add(index);
            add(titleId);
            add(kind);
            add(region);
            add(name);
        }

        Gtk::TreeModelColumn<int> index;
        Gtk::TreeModelColumn<Glib::ustring> titleId;
        Gtk::TreeModelColumn<Glib::ustring> kind;
        Gtk::TreeModelColumn<Glib::ustring> region;
        Gtk::TreeModelColumn<Glib::ustring> name;
    };

    GameList(Glib::RefPtr<Gtk::Builder> builder, const TitleEntry *infos);
    ~GameList();

    void updateTitles(TITLE_CATEGORY cat);

    void on_gamelist_row_activated(const Gtk::TreePath& treePath, Gtk::TreeViewColumn* const& column);
    void on_game_button_selected(GdkEventButton* ev);
    void on_update_button_selected(GdkEventButton* ev);
    void on_dlc_button_selected(GdkEventButton* ev);
    void on_demo_button_selected(GdkEventButton* ev);
    void on_all_button_selected(GdkEventButton* ev);
    void on_dumpWindow_closed();
    bool on_search_equal(const Glib::RefPtr<Gtk::TreeModel>& model, int column, const Glib::ustring& key, const Gtk::TreeModel::iterator& iter);

    Gtk::Window* getWindow() { return gameListWindow; }

private:
    Gtk::Window* gameListWindow = nullptr;
    Glib::RefPtr<Gtk::Builder> builder;
    std::vector<uint8_t> key;

    Gtk::TreeView* treeView = nullptr;
    Gtk::RadioButton *gamesButton = nullptr;
    Gtk::RadioButton *updatesButton = nullptr;
    Gtk::RadioButton *dlcsButton = nullptr;
    Gtk::RadioButton *demosButton = nullptr;
    Gtk::RadioButton *allButton = nullptr;
    ModelColumns columns;
    const TitleEntry *infos;

    sigc::connection deleteConn;

    Gtk::AboutDialog* aboutDialog = nullptr;
    sigc::connection aboutConn;
};