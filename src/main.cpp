#include <gtkmm.h>

#include <fcntl.h>

#ifdef _WIN32
#include <registry.h>
#include <updater.h>
#include <windows.h>
#endif

#include <GameList.h>
#include <utils.h>

int main(int argc, char *argv[]) {
#ifdef _WIN32
    FreeConsole();
    if (AttachConsole(ATTACH_PARENT_PROCESS))
        AllocConsole();
#endif // _WIN32
    close(STDOUT_FILENO);
    close(STDERR_FILENO);

    int log = open("log.txt", O_CREAT | O_RDWR, 0644);

    dup2(log, STDOUT_FILENO);
    dup2(log, STDERR_FILENO);

    Glib::RefPtr<Gtk::Application> app = Gtk::Application::create(argc, argv, "org.gtkmm.example");
    Glib::RefPtr<Gtk::Builder> builder = Gtk::Builder::create_from_resource("/wiiudownloader/data/wiiudownloader.ui");

#ifdef _WIN32
    checkAndDownloadLatestVersion();
    checkAndEnableLongPaths();
#endif // _WIN32

    GameList *list = new GameList(app, builder, getTitleEntries(TITLE_CATEGORY_GAME));

    list->getWindow()->set_title("WiiUDownloader");

    setGameList(list->getWindow()->gobj());

    app->run(*list->getWindow());

    delete list->getWindow();
    delete list;

    return 0;
}