#pragma once

#include <gtkmm.h>

class Window : public Gtk::Window {
  public:
    Window();
    virtual ~Window();

  protected:
    void on_button_clicked();

    Gtk::Fixed fixed;
    Gtk::ScrolledWindow scrolledWindow;

    Gtk::Button generateButton;
    Glib::RefPtr<Gtk::Builder> builder;
};