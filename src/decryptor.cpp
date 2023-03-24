#include <decryptor.h>
#include <functional>
#include <gtk/gtk.h>
#include <libgodecrypt.h>
#include <thread>

static GtkWidget *window;

static gboolean pulseBar(gpointer data) {
    gdk_threads_enter();
    gtk_progress_bar_pulse(GTK_PROGRESS_BAR(data));
    gdk_threads_leave();

    return TRUE;
}

static gboolean destroyWindow(gpointer user_data) {
    GtkWidget *window = GTK_WIDGET(user_data);
    gtk_widget_destroy(window);
    return G_SOURCE_REMOVE;
}

static void destroyWindowFromThread(GtkWidget *window) {
    GSource *source = g_idle_source_new();
    g_source_set_callback(source, destroyWindow, window, NULL);
    g_source_attach(source, NULL);
}

static void decryptionDialog() {
    gtk_init(NULL, NULL);

    //Create window
    window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "Decryption Progress");
    gtk_window_set_default_size(GTK_WINDOW(window), 300, 50);
    gtk_container_set_border_width(GTK_CONTAINER(window), 10);
    gtk_window_set_modal(GTK_WINDOW(window), TRUE);

    //Create progress bar
    GtkWidget *progress_bar = gtk_progress_bar_new();
    gtk_progress_bar_set_show_text(GTK_PROGRESS_BAR(progress_bar), TRUE);
    gtk_progress_bar_set_text(GTK_PROGRESS_BAR(progress_bar), "Decrypting...");
    gint pulse_ref = g_timeout_add(300, pulseBar, progress_bar);
    g_object_set_data(G_OBJECT(progress_bar), "pulse_id",
                      GINT_TO_POINTER(pulse_ref));

    //Create container for the window
    GtkWidget *main_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 5);
    gtk_container_add(GTK_CONTAINER(window), main_box);
    gtk_box_pack_start(GTK_BOX(main_box), progress_bar, FALSE, FALSE, 0);

    gtk_widget_show_all(window);
}

void workerFunction(char *path, std::function<void()> callback) {
    DecryptAndExtract(path);
    callback();
}

void decryptor(const char *path, bool showProgressDialog) {
    if (showProgressDialog)
        decryptionDialog();

    std::thread decryptThread(workerFunction, const_cast<char *>(path), [&] {
        if (showProgressDialog)
            destroyWindowFromThread(window);
    });

    decryptThread.detach();
}