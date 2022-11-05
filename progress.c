// C++ show progress of libcurl download in GTK 3 progress bar


#include <gtk/gtk.h>
#include <curl/curl.h>

#include <stdio.h>

GtkWidget *progress_bar;

void destroy(GtkWidget *widget, gpointer data)
{
    gtk_main_quit();
}

int xferinfo(void *p, curl_off_t dltotal, curl_off_t dlnow, curl_off_t ultotal, curl_off_t ulnow)
{
    gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress_bar), (gdouble)dlnow/(gdouble)dltotal);
    while (gtk_events_pending()) {
        gtk_main_iteration();
    }
    return 0;
}

int main(int argc, char *argv[])
{
    gtk_init(&argc, &argv);

    GtkWidget *window;
    GtkWidget *vbox;

    window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "Libcurl Progress");
    gtk_window_set_default_size(GTK_WINDOW(window), 400, 100);
    gtk_container_set_border_width(GTK_CONTAINER(window), 10);
    g_signal_connect(window, "destroy", G_CALLBACK(destroy), NULL);

    vbox = gtk_vbox_new(FALSE, 0);
    gtk_container_add(GTK_CONTAINER(window), vbox);

    progress_bar = gtk_progress_bar_new();
    gtk_box_pack_start(GTK_BOX(vbox), progress_bar, TRUE, TRUE, 5);

    gtk_widget_show_all(window);

    CURL *curl;
    CURLcode res;

    curl = curl_easy_init();
    if (curl) {
        curl_easy_setopt(curl, CURLOPT_URL, "http://www.example.com/");
        curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, 1L);
        curl_easy_setopt(curl, CURLOPT_NOPROGRESS, 0L);
        curl_easy_setopt(curl, CURLOPT_XFERINFOFUNCTION, xferinfo);
        res = curl_easy_perform(curl);
        if (res != CURLE_OK) {
            gtk_main_quit();
        }

        curl_easy_cleanup(curl);
    }

    gtk_main();

    return 0;
}