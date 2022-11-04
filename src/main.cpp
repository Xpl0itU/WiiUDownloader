#include <window.h>
#include <iostream>
#include <gtkmm/application.h>

#include <GameList.h>

Window::Window() {
  add(scrolledWindow);
  scrolledWindow.add(fixed);

  generateButton.add_label("Start");

  set_title("Wii U Downloader");
  set_border_width(10);

  generateButton.signal_clicked().connect(sigc::mem_fun(*this, &Window::on_button_clicked));

  fixed.add(generateButton);

  fixed.move(generateButton, 90, 245);
  
  resize(300, 300);

  show_all();
}

Window::~Window() {}

void Window::on_button_clicked() {
  builder = Gtk::Builder::create_from_file("dumpsteru.ui");

  GameList *list = new GameList(builder, getTitleEntries(TITLE_CATEGORY_ALL));
}

int main(int argc, char *argv[]) {
  auto app = Gtk::Application::create(argc, argv, "org.gtkmm.example");

  Window window;

  return app->run(window);
}