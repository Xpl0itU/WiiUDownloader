// C show progress of libcurl download in GTK 3 progress bar from a new pthread



#include <gtk-3.0/gtk/gtk.h>
#include <stdio.h>
#include <curl/curl.h>
#include <pthread.h>


GtkWidget *progress_bar;
GtkWidget *button;
GtkWidget *window;

//Curl options
const char *url = "http://download.inspire.net.nz/data/1GB.zip"; //download url
const char *file = "file.html"; //file path
CURLcode res;

//Thread
pthread_t thread;
int thread_running = 0;

//LibCurl progress function
int progress_func(void *ptr, double t, double d, double ultotal, double ulnow)
{
    if(t == 0) {
        gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress_bar), 0);
        return 0;
    }

    gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress_bar), d/t);
    while(gtk_events_pending())
        gtk_main_iteration();

    return 0;
}

static size_t write_data(void* data, size_t size, size_t nmemb, void* file_stream)
{
    size_t written = fwrite(data, size, nmemb, (FILE*)file_stream);
    return written;
}

//LibCurl download function
void *download(void *ptr)
{
    FILE *fp;
    CURL *curl = curl_easy_init();
    if (curl) {
        fp = fopen(file, "wb");
        curl_easy_setopt(curl, CURLOPT_URL, url);
        curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_data);
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, fp);
        curl_easy_setopt(curl, CURLOPT_NOPROGRESS, 0L);
        curl_easy_setopt(curl, CURLOPT_PROGRESSFUNCTION, progress_func);
        curl_easy_setopt(curl, CURLOPT_PROGRESSDATA, progress_bar);
        res = curl_easy_perform(curl);
        fclose(fp);
        curl_easy_cleanup(curl);
    }
    thread_running = 0;
    return NULL;
}


//Start download function
void start_download(GtkWidget *widget, gpointer data)
{
    if(thread_running)
        return;

    thread_running = 1;
    if(pthread_create(&thread, NULL, download, NULL)) {
        fprintf(stderr, "Error creating thread\n");
        thread_running = 0;
        return;
    }
}

int main(int argc, char *argv[])
{
    gtk_init(&argc, &argv);

    //Create window
    window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "LibCurl Download Progress Bar");
    gtk_window_set_default_size(GTK_WINDOW(window), 300, 50);
    gtk_container_set_border_width(GTK_CONTAINER(window), 10);
    g_signal_connect(window, "destroy", G_CALLBACK(gtk_main_quit), NULL);

    //Create progress bar
    progress_bar = gtk_progress_bar_new();
    gtk_progress_bar_set_show_text(GTK_PROGRESS_BAR(progress_bar), TRUE);
    gtk_progress_bar_set_text(GTK_PROGRESS_BAR(progress_bar), "Downloading");

    //Create container for the window
    GtkWidget *main_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 5);
    gtk_container_add(GTK_CONTAINER(window), main_box);
    gtk_box_pack_start(GTK_BOX(main_box), progress_bar, FALSE, FALSE, 0);

    gtk_widget_show_all(window);

    start_download(window, NULL);

    gtk_main();

    return 0;
}