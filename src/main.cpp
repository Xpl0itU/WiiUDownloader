#include <gtkmm.h>
#include <iostream>

#include <fcntl.h>

#include <GameList.h>

int main(int argc, char *argv[]) {
    close(STDOUT_FILENO);
    close(STDERR_FILENO);

    int log = open("log.txt", O_CREAT | O_RDWR, 0644);

    dup2(log, STDOUT_FILENO);
    dup2(log, STDERR_FILENO);

    Glib::RefPtr<Gtk::Application> app = Gtk::Application::create(argc, argv, "org.gtkmm.example");
    Glib::RefPtr<Gtk::Builder> builder = Gtk::Builder::create_from_resource("/data/wiiudownloader.ui");

    GameList *list = new GameList(builder, getTitleEntries(TITLE_CATEGORY_GAME));

    app->run(*list->getWindow());

    delete list->getWindow();
    delete list;

    return 0;
}