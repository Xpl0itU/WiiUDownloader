#include <iostream>
#include <gtkmm.h>

#include <GameList.h>

int main(int argc, char *argv[]) {

  Glib::RefPtr<Gtk::Application> app = Gtk::Application::create(argc, argv, "org.gtkmm.example");
  Glib::RefPtr<Gtk::Builder> builder = Gtk::Builder::create_from_resource("/data/wiiudownloader.ui");

  GameList *list = new GameList(builder, getTitleEntries(TITLE_CATEGORY_ALL));

  app->run(*list->getWindow());

  delete list->getWindow();
  delete list;

  return 0;
}