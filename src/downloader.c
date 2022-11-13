#include <inttypes.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#ifndef _WIN32
#include <sys/stat.h>
#endif

#include <downloader.h>
#include <keygen.h>

#include <curl/curl.h>
#include <gtk/gtk.h>

#include <pthread.h>

struct MemoryStruct {
    uint8_t *memory;
    size_t size;
};

struct PathFileStruct {
    char *file_path;
    FILE *file_pointer;
};

GtkWidget *progress_bar;
GtkWidget *window;

char currentFile[255] = "None";
char *selected_dir = NULL;

uint16_t bswap_16(uint16_t value) {
    return (uint16_t) ((0x00FF & (value >> 8)) | (0xFF00 & (value << 8)));
}

static inline uint32_t bswap_32(uint32_t __x) {
    return __x >> 24 | __x >> 8 & 0xff00 | __x << 8 & 0xff0000 | __x << 24;
}

char *readable_fs(double size, char *buf) {
    int i = 0;
    const char *units[] = {"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"};
    while (size > 1024) {
        size /= 1024;
        i++;
    }
    sprintf(buf, "%.*f %s", i, size, units[i]);
    return buf;
}

//LibCurl progress function
int progress_func(void *p,
                    curl_off_t dltotal, curl_off_t dlnow,
                    curl_off_t ultotal, curl_off_t ulnow) {
    if (dltotal == 0)
        dltotal = 1;
    if (dlnow == 0)
        dlnow = 1;
    GtkProgressBar *progress_bar = (GtkProgressBar *) p;

    char downloadString[255];
    char downNow[255];
    char downTotal[255];
    readable_fs(dlnow, downNow);
    readable_fs(dltotal, downTotal);
    sprintf(downloadString, "Downloading %s (%s/%s)", currentFile, downNow, downTotal);

    gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress_bar), (double)dlnow / (double)dltotal);
    gtk_progress_bar_set_text(GTK_PROGRESS_BAR(progress_bar), downloadString);
    // force redraw
    while (gtk_events_pending())
        gtk_main_iteration();
    return 0;
}

static size_t WriteDataToMemory(void *contents, size_t size, size_t nmemb, void *userp) {
    size_t realsize = size * nmemb;
    struct MemoryStruct *mem = (struct MemoryStruct *) userp;

    mem->memory = realloc(mem->memory, mem->size + realsize);
    memcpy(&(mem->memory[mem->size]), contents, realsize);
    mem->size += realsize;

    return realsize;
}

void create_ticket(const char *title_id, const char *title_key, uint16_t title_version, const char *output_path) {
    FILE *ticket_file = fopen(output_path, "wb");
    if (!ticket_file) {
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }

    uint8_t ticket_data[848] = "\x00\x01\x00\x04\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x52\x6f\x6f\x74\x2d\x43\x41\x30\x30\x30\x30\x30\x30\x30\x33\x2d\x58\x53\x30\x30\x30\x30\x30\x30\x30\x63\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\x01\x00\x00\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xaa\xaa\xaa\xaa\xaa\xaa\xaa\xaa\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x14\x00\x00\x00\xac\x00\x00\x00\x14\x00\x01\x00\x14\x00\x00\x00\x00\x00\x00\x00\x28\x00\x00\x00\x01\x00\x00\x00\x84\x00\x00\x00\x84\x00\x03\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00";
    // fill in the values from the title_id and title_key
    hex2bytes(title_id, &ticket_data[476]);
    hex2bytes(title_key, &ticket_data[447]);
    memcpy(&ticket_data[486], &title_version, 2);
    fwrite(ticket_data, 1, 848, ticket_file);
    fclose(ticket_file);
    printf("Finished creating \"%s\".\n", output_path);
}

void downloadCert(const char *outputPath) {
    FILE *cetk = fopen(outputPath, "wb");
    if(cetk == NULL)
        return;
    CURL *certHandle = curl_easy_init();
    curl_easy_setopt(certHandle, CURLOPT_FAILONERROR, 1L);

    // Download the tmd and save it in memory, as we need some data from it
    curl_easy_setopt(certHandle, CURLOPT_WRITEFUNCTION, fwrite);
    curl_easy_setopt(certHandle, CURLOPT_URL, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/000500101000400A/cetk");
    
    curl_easy_setopt(certHandle, CURLOPT_WRITEDATA, cetk);
    curl_easy_perform(certHandle);
    curl_easy_cleanup(certHandle);
    fclose(cetk);
}

void progressDialog() {
    gtk_init(NULL, NULL);

    //Create window
    window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), "Download Progress");
    gtk_window_set_default_size(GTK_WINDOW(window), 300, 50);
    gtk_container_set_border_width(GTK_CONTAINER(window), 10);
    gtk_window_set_modal(GTK_WINDOW(window), TRUE);

    //Create progress bar
    progress_bar = gtk_progress_bar_new();
    gtk_progress_bar_set_show_text(GTK_PROGRESS_BAR(progress_bar), TRUE);
    gtk_progress_bar_set_text(GTK_PROGRESS_BAR(progress_bar), "Downloading");

    //Create container for the window
    GtkWidget *main_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 5);
    gtk_container_add(GTK_CONTAINER(window), main_box);
    gtk_box_pack_start(GTK_BOX(main_box), progress_bar, FALSE, FALSE, 0);

    gtk_widget_show_all(window);
}

int downloadFile(const char *download_url, const char *output_path) {
    FILE *file = fopen(output_path, "wb");
    if(file == NULL)
        return 1;
    CURL *handle = curl_easy_init();
    curl_easy_setopt(handle, CURLOPT_FAILONERROR, 1L);

    // Download the tmd and save it in memory, as we need some data from it
    curl_easy_setopt(handle, CURLOPT_WRITEFUNCTION, fwrite);
    curl_easy_setopt(handle, CURLOPT_URL, download_url);
    curl_easy_setopt(handle, CURLOPT_NOPROGRESS, 0L);
    curl_easy_setopt(handle, CURLOPT_XFERINFOFUNCTION, progress_func);
    curl_easy_setopt(handle, CURLOPT_PROGRESSDATA, progress_bar);
    
    curl_easy_setopt(handle, CURLOPT_WRITEDATA, file);
    curl_easy_perform(handle);
    curl_easy_cleanup(handle);
    fclose(file);
    return 0;
}

// function to return the path of the selected folder
char *gtk3_show_folder_select_dialog() {
    GtkWidget *dialog;
    GtkFileChooserAction action = GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER;
    gint res;
    char *folder_path = NULL;

    dialog = gtk_file_chooser_dialog_new("Select a folder",
                                         NULL,
                                         action,
                                         "_Cancel",
                                         GTK_RESPONSE_CANCEL,
                                         "_Select",
                                         GTK_RESPONSE_ACCEPT,
                                         NULL);

    res = gtk_dialog_run(GTK_DIALOG(dialog));

    if (res == GTK_RESPONSE_ACCEPT) {
        GtkFileChooser *chooser = GTK_FILE_CHOOSER(dialog);
        folder_path = gtk_file_chooser_get_filename(chooser);
    }

    gtk_widget_destroy(dialog);

    return folder_path;
}

void prepend(char *s, const char *t) {
    size_t len = strlen(t);
    memmove(s + len, s, strlen(s) + 1);
    memcpy(s, t, len);
}

int downloadTitle(const char *titleID, bool decrypt) {
    // initialize some useful variables
    char *output_dir = malloc(1024);
    strcpy(output_dir, titleID);
    prepend(output_dir, "/");
    if (selected_dir == NULL)
        selected_dir = gtk3_show_folder_select_dialog();
    if (selected_dir == NULL) {
        free(output_dir);
        return 0;
    }
    prepend(output_dir, selected_dir);
    if (output_dir[strlen(output_dir) - 1] == '/' || output_dir[strlen(output_dir) - 1] == '\\') {
        output_dir[strlen(output_dir) - 1] = '\0';
    }
    char base_url[69];
    snprintf(base_url, 69, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID);
    char download_url[81];
    char output_path[strlen(output_dir) + 14];

// create the output directory if it doesn't exist
#ifdef _WIN32
    mkdir(output_dir);
#else
    mkdir(output_dir, 0700);
#endif

    // initialize curl
    curl_global_init(CURL_GLOBAL_ALL);

    // make an own handle for the tmd file, as we wanna download it to memory first
    CURL *tmd_handle = curl_easy_init();
    curl_easy_setopt(tmd_handle, CURLOPT_FAILONERROR, 1L);

    // Download the tmd and save it in memory, as we need some data from it
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEFUNCTION, WriteDataToMemory);
    snprintf(download_url, 73, "%s/%s", base_url, "tmd");
    curl_easy_setopt(tmd_handle, CURLOPT_URL, download_url);

    struct MemoryStruct tmd_data;
    tmd_data.memory = malloc(0);
    tmd_data.size = 0;
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEDATA, (void *) &tmd_data);
    curl_easy_perform(tmd_handle);
    curl_easy_cleanup(tmd_handle);
    // write out the tmd file
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.cert");
    downloadCert(output_path);
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tmd");
    FILE *tmd_file = fopen(output_path, "wb");
    if (!tmd_file) {
        free(output_dir);
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }
    fwrite(tmd_data.memory, 1, tmd_data.size, tmd_file);
    fclose(tmd_file);
    printf("Finished downloading \"%s\".\n", output_path);

    uint16_t title_version;
    memcpy(&title_version, &tmd_data.memory[476], 2);
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tik");
    char titleKey[128];
    generateKey(titleID, titleKey);
    create_ticket(titleID, titleKey, title_version, output_path);

    uint16_t content_count;
    memcpy(&content_count, &tmd_data.memory[478], 2);
    content_count = bswap_16(content_count);

    // Add all needed curl handles to the multi handle
    progressDialog();
    for (int i = 0; i < content_count; i++) {
        int offset = 2820 + (48 * i);
        uint32_t id; // the id should usually be chronological, but we wanna be sure
        memcpy(&id, &tmd_data.memory[offset], 4);
        id = bswap_32(id);

        // add a curl handle for the content file (.app file)
        snprintf(output_path, sizeof(output_path), "%s/%08X.app", output_dir, id);
        snprintf(download_url, 78, "%s/%08X", base_url, id);
        sprintf(currentFile, "%08X.app", id);
        downloadFile(download_url, output_path);

        if ((tmd_data.memory[offset + 7] & 0x2) == 2) {
            // add a curl handle for the hash file (.h3 file)
            snprintf(output_path, sizeof(output_path), "%s/%08X.h3", output_dir, id);
            snprintf(download_url, 81, "%s/%08X.h3", base_url, id);
            sprintf(currentFile, "%08X.h3", id);
            downloadFile(download_url, output_path);
        }
    }
    free(tmd_data.memory);

    printf("Downloading all files for TitleID %s done...\n", titleID);

    // cleanup curl stuff
    gtk_widget_destroy(GTK_WIDGET(window));
    curl_global_cleanup();
    free(output_dir);
}