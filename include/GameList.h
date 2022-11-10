#pragma once

#include <gtkmm.h>
#include <string>
#include <vector>

#include <keygen.h>
#include <gtitles.h>
#include <titleInfo.h>

class GameList
{
public:
    class ModelColumns : public Gtk::TreeModel::ColumnRecord
    {
    public:
        ModelColumns()
        {
            add(index);
            add(toQueue);
            add(titleId);
            add(kind);
            add(region);
            add(name);
        }

        Gtk::TreeModelColumn<int> index;
        Gtk::TreeModelColumn<bool> toQueue;
        Gtk::TreeModelColumn<Glib::ustring> titleId;
        Gtk::TreeModelColumn<Glib::ustring> kind;
        Gtk::TreeModelColumn<Glib::ustring> region;
        Gtk::TreeModelColumn<Glib::ustring> name;
    };

    GameList(Glib::RefPtr<Gtk::Builder> builder, const TitleEntry *infos);
    ~GameList();

    void updateTitles(TITLE_CATEGORY cat, MCPRegion reg);

    void on_gamelist_row_activated(const Gtk::TreePath& treePath, Gtk::TreeViewColumn* const& column);
    void on_button_selected(GdkEventButton* ev, TITLE_CATEGORY cat);
    void on_region_selected(Gtk::ToggleButton* button, MCPRegion reg);
    void on_add_to_queue(GdkEventButton* ev);
    void on_selection_changed();
    void on_download_queue(GdkEventButton* ev);
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
    Gtk::CheckButton *japanButton = nullptr;
    Gtk::CheckButton *usaButton = nullptr;
    Gtk::CheckButton *europeButton = nullptr;
    Gtk::Button *addToQueueButton = nullptr;
    Gtk::Button *downloadQueueButton = nullptr;
    ModelColumns columns;
    const TitleEntry *infos;

    std::vector<uint64_t> queueVector = {};

    TITLE_CATEGORY currentCategory = TITLE_CATEGORY_GAME;
    Glib::RefPtr<Gtk::ListStore> treeModel;

    sigc::connection deleteConn;

    MCPRegion selectedRegion = MCP_REGION_JAPAN | MCP_REGION_USA | MCP_REGION_EUROPE;
};