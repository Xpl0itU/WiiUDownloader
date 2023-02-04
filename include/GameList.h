#pragma once

#include <gtkmm.h>
#include <map>
#include <string>
#include <vector>

#include <gtitles.h>
#include <keygen.h>
#include <titleInfo.h>

class GameList {
public:
    class ModelColumns : public Gtk::TreeModel::ColumnRecord {
    public:
        ModelColumns() {
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

    void on_gamelist_row_activated(const Gtk::TreePath &treePath, Gtk::TreeViewColumn *const &column);
    void on_button_selected(GdkEventButton *ev, TITLE_CATEGORY cat);
    void on_region_selected(Gtk::ToggleButton *button, MCPRegion reg);
    void on_add_to_queue(GdkEventButton *ev);
    void on_selection_changed();
    void on_download_queue(GdkEventButton *ev);
    void on_decrypt_selected(Gtk::ToggleButton *button);
    void on_delete_encrypted_selected(Gtk::ToggleButton *button);
    bool on_search_equal(const Glib::RefPtr<Gtk::TreeModel> &model, int column, const Glib::ustring &key, const Gtk::TreeModel::iterator &iter);
    void search_entry_changed();
    void on_decrypt_menu_click();
    void on_generate_fake_tik_menu_click();

    Gtk::Window *getWindow() { return gameListWindow; }

private:
    Gtk::Window *gameListWindow = nullptr;
    Glib::RefPtr<Gtk::Builder> builder;

    Gtk::TreeView *treeView = nullptr;
    Gtk::RadioButton *gamesButton = nullptr;
    Gtk::RadioButton *updatesButton = nullptr;
    Gtk::RadioButton *dlcsButton = nullptr;
    Gtk::RadioButton *demosButton = nullptr;
    Gtk::RadioButton *allButton = nullptr;
    Gtk::CheckButton *japanButton = nullptr;
    Gtk::CheckButton *usaButton = nullptr;
    Gtk::CheckButton *europeButton = nullptr;
    Gtk::CheckButton *decryptContentsButton = nullptr;
    Gtk::CheckButton *deleteEncryptedContentsButton = nullptr;
    Gtk::Button *addToQueueButton = nullptr;
    Gtk::Button *downloadQueueButton = nullptr;
    ModelColumns columns;
    const TitleEntry *infos;

    bool *cancelQueue = (bool *) malloc(1);

    Glib::RefPtr<Gtk::TreeModelFilter> m_refTreeModelFilter;
    Gtk::SearchBar *searchBar = nullptr;
    Gtk::SearchEntry *searchEntry = nullptr;

    bool decryptContents = false;
    bool deleteEncryptedContents = false;

    std::map<uint64_t, const char *> queueMap = {};

    TITLE_CATEGORY currentCategory = TITLE_CATEGORY_GAME;
    Glib::RefPtr<Gtk::ListStore> treeModel;

    MCPRegion selectedRegion = (MCPRegion) (MCP_REGION_JAPAN | MCP_REGION_USA | MCP_REGION_EUROPE);
};