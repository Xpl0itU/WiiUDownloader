#include <GameList.h>

#include <cstdlib>
#include <downloader.h>
#include <iostream>

typedef enum MCPRegion
{
   MCP_REGION_JAPAN                    = 0x01,
   MCP_REGION_USA                      = 0x02,
   MCP_REGION_EUROPE                   = 0x04,
   MCP_REGION_CHINA                    = 0x10,
   MCP_REGION_KOREA                    = 0x20,
   MCP_REGION_TAIWAN                   = 0x40,
} MCPRegion;

typedef enum
    {
        TID_HIGH_GAME = 0x00050000,
        TID_HIGH_DEMO = 0x00050002,
        TID_HIGH_SYSTEM_APP = 0x00050010,
        TID_HIGH_SYSTEM_DATA = 0x0005001B,
        TID_HIGH_SYSTEM_APPLET = 0x00050030,
        TID_HIGH_VWII_IOS = 0x00000007,
        TID_HIGH_VWII_SYSTEM_APP = 0x00070002,
        TID_HIGH_VWII_SYSTEM = 0x00070008,
        TID_HIGH_DLC = 0x0005000C,
        TID_HIGH_UPDATE = 0x0005000E,
    } TID_HIGH;

#define getTidHighFromTid(tid) ((uint32_t)(tid >> 32))

const char* getFormattedKind(uint64_t tid) {
    uint32_t highID = getTidHighFromTid(tid);
    switch(highID) {
        case TID_HIGH_GAME:
            return "Game";
            break;
        case TID_HIGH_DEMO:
            return "Demo";
            break;
        case TID_HIGH_SYSTEM_APP:
            return "System App";
            break;
        case TID_HIGH_SYSTEM_DATA:
            return "System Data";
            break;
        case TID_HIGH_SYSTEM_APPLET:
            return "System Applet";
            break;
        case TID_HIGH_VWII_IOS:
            return "vWii IOS";
            break;
        case TID_HIGH_VWII_SYSTEM_APP:
            return "vWii System App";
            break;
        case TID_HIGH_VWII_SYSTEM:
            return "vWii System";
            break;
        case TID_HIGH_DLC:
            return "DLC";
            break;
        case TID_HIGH_UPDATE:
            return "Update";
            break;
        default:
            return "Unknown";
            break;
    }
}

const char *getFormattedRegion(MCPRegion region)
{
    if(region & MCP_REGION_EUROPE)
    {
        if(region & MCP_REGION_USA)
            return region & MCP_REGION_JAPAN ? "All" : "USA/Europe";

        return region & MCP_REGION_JAPAN ? "Europe/Japan" : "Europe";
    }

    if(region & MCP_REGION_USA)
        return region & MCP_REGION_JAPAN ? "USA/Japan" : "USA";

    return region & MCP_REGION_JAPAN ? "Japan" : "Unknown";
}

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
        row[columns.region] = Glib::ustring::format(getFormattedRegion(infos[i].region));
        row[columns.kind] = Glib::ustring::format(getFormattedKind(infos[i].tid));
        row[columns.titleId] = Glib::ustring::format(id);
    }

    treeView->append_column("TitleID", columns.titleId);
    treeView->get_column(1);
    treeView->get_column(1);

    treeView->append_column("Kind", columns.kind);
    treeView->get_column(2);
    treeView->get_column(2);

    treeView->append_column("Region", columns.region);
    treeView->get_column(3);
    treeView->get_column(3);

    treeView->append_column("Name", columns.name);
    treeView->get_column(4);
    treeView->get_column(4);
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