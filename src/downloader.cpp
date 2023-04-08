#include <curl/curl.h>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <future>
#include <unistd.h>
#ifndef _WIN32
#include <sys/stat.h>
#endif

#include <cdecrypt/cdecrypt.h>
#include <downloader.h>
#include <fst.h>
#include <keygen.h>
#include <log.h>
#include <nfd.h>
#include <settings.h>
#include <tmd.h>
#include <utils.h>

#include "cdecrypt/util.h"

#define MAX_RETRIES 5

struct MemoryStruct {
    uint8_t *memory;
    size_t size;
};

struct CURLProgress {
    size_t titleSize;
    size_t downloadedSize;
    size_t previousDownloadedSize;
    char *totalSize;
    char *currentFile;
};

static GtkWidget *window;

static char *selected_dir = NULL;
static bool cancelled = false;
static bool paused = false;
static bool queueCancelled;
static bool downloadWiiVC = false;

static struct CURLProgress *progress = NULL;
static char *currentTitle = NULL;
static GtkWidget *progress_bar = NULL;
static GtkWidget *gameLabel = NULL;
static CURL *handle = NULL;

static CURLSH *share = NULL;

static char *readable_fs(double size, char *buf) {
    int i = 0;
    const char *units[] = {"B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"};
    while (size > 1024) {
        size /= 1024;
        i++;
    }
    sprintf(buf, "%.*f %s", i, size, units[i]);
    return buf;
}

static size_t write_function(void *data, size_t size, size_t nmemb, void *userp) {
    size_t written = fwrite(data, size, nmemb, (FILE *)userp);
    return cancelled ? 0 : written;
}

static void cancel_button_clicked(GtkWidget *widget, gpointer data) {
    cancelled = true;
    setQueueCancelled(true);
}

static void pause_button_clicked(GtkWidget *widget, gpointer data) {
    struct CURLProgress *progress = (struct CURLProgress *) data;
    if (paused) {
        curl_easy_pause(handle, CURLPAUSE_CONT);
        gtk_button_set_label(GTK_BUTTON(widget), "Pause");
    } else {
        curl_easy_pause(handle, CURLPAUSE_ALL);
        gtk_button_set_label(GTK_BUTTON(widget), "Resume");
    }
    paused = !paused;
}

static void hide_button_clicked(GtkWidget *widget, gpointer data) {
    gtk_window_iconify(GTK_WINDOW(window));
    minimizeGameListWindow();
}

//LibCurl progress function
int progress_func(void *p,
                  curl_off_t dltotal, curl_off_t dlnow,
                  curl_off_t ultotal, curl_off_t ulnow) {
    if (dltotal == 0)
        dltotal = 1;
    if (dlnow == 0)
        dlnow = 1;
    struct CURLProgress *progress = (struct CURLProgress *) p;

    char *downloadString = (char *) malloc(1024);
    char *speedString = (char *) malloc(255);
    char *downNow = (char *) malloc(255);
    progress->downloadedSize -= progress->previousDownloadedSize;
    progress->downloadedSize += dlnow;
    readable_fs(progress->downloadedSize, downNow);
    double speed;
    curl_easy_getinfo(handle, CURLINFO_SPEED_DOWNLOAD, &speed);
    readable_fs(speed, speedString);
    strcat(speedString, "/s");
    sprintf(downloadString, "Downloading %s (%s/%s) (%s)", progress->currentFile, downNow, progress->totalSize, speedString);

    gtk_progress_bar_set_fraction(GTK_PROGRESS_BAR(progress_bar), (double) progress->downloadedSize / (double) progress->titleSize);
    gtk_progress_bar_set_text(GTK_PROGRESS_BAR(progress_bar), downloadString);
    // force redraw
    while (gtk_events_pending())
        gtk_main_iteration();

    progress->previousDownloadedSize = dlnow;
    free(downloadString);
    free(speedString);
    free(downNow);
    return 0;
}

static size_t WriteDataToMemory(void *contents, size_t size, size_t nmemb, void *userp) {
    size_t realsize = size * nmemb;
    struct MemoryStruct *mem = (struct MemoryStruct *) userp;

    mem->memory = (uint8_t *)realloc(mem->memory, mem->size + realsize);
    memcpy(&(mem->memory[mem->size]), contents, realsize);
    mem->size += realsize;

    return realsize;
}

void progressDialog() {
    gtk_init(NULL, NULL);
    GtkWidget *cancelButton = gtk_button_new();
    GtkWidget *pauseButton = gtk_button_new();
    GtkWidget *hideButton = gtk_button_new();
    gameLabel = gtk_label_new(currentTitle);

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

    gtk_button_set_label(GTK_BUTTON(cancelButton), "Cancel");
    g_signal_connect(cancelButton, "clicked", G_CALLBACK(cancel_button_clicked), NULL);

    gtk_button_set_label(GTK_BUTTON(pauseButton), "Pause");
    g_signal_connect(pauseButton, "clicked", G_CALLBACK(pause_button_clicked), progress);

    gtk_button_set_label(GTK_BUTTON(hideButton), "Hide");
    g_signal_connect(hideButton, "clicked", G_CALLBACK(hide_button_clicked), NULL);

    //Create container for the window
    GtkWidget *main_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 5);
    gtk_container_add(GTK_CONTAINER(window), main_box);
    gtk_box_pack_start(GTK_BOX(main_box), gameLabel, FALSE, FALSE, 0);
    gtk_box_pack_start(GTK_BOX(main_box), progress_bar, FALSE, FALSE, 0);
    GtkWidget *button_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 5);
    gtk_container_add(GTK_CONTAINER(main_box), button_box);
    gtk_box_pack_start(GTK_BOX(button_box), hideButton, FALSE, FALSE, 0);
    gtk_box_pack_end(GTK_BOX(button_box), cancelButton, FALSE, FALSE, 0);
    gtk_box_pack_end(GTK_BOX(button_box), pauseButton, FALSE, FALSE, 0);

    gtk_widget_show_all(window);
}

void destroyProgressDialog() {
    gtk_widget_destroy(GTK_WIDGET(window));
}

GtkWidget *getProgressBar() {
    return progress_bar;
}

static int compareRemoteFileSize(const char *url, const char *local_file) {
    CURL *curl;
    CURLcode res;
    double remote_filesize = 0;
    double local_filesize = 0;

    curl = curl_easy_init();
    if (curl) {
        curl_easy_setopt(curl, CURLOPT_URL, url);
        curl_easy_setopt(curl, CURLOPT_HEADER, 1L);
        curl_easy_setopt(curl, CURLOPT_NOBODY, 1L);
        if (share != NULL)
            curl_easy_setopt(curl, CURLOPT_SHARE, share);
        res = curl_easy_perform(curl);
        if (res == CURLE_OK) {
            res = curl_easy_getinfo(curl, CURLINFO_CONTENT_LENGTH_DOWNLOAD, &remote_filesize);
            if (res == CURLE_OK) {
                FILE *fp = fopen(local_file, "rb");
                if (fp) {
                    fseek(fp, 0L, SEEK_END);
                    local_filesize = ftell(fp);
                    fclose(fp);
                }
                if (remote_filesize == local_filesize) {
                    return 0;
                } else if (remote_filesize > local_filesize) {
                    return 1;
                } else {
                    return -1;
                }
            }
        }
        curl_easy_cleanup(curl);
    }
    return -2;
}

static int downloadFile(const char *download_url, const char *output_path, struct CURLProgress *progress, bool doRetrySleep) {
    progress->previousDownloadedSize = 0;
    if (fileExists(output_path)) {
        if (compareRemoteFileSize(download_url, output_path) == 0) {
            log_info("The file already exists and has the same or bigger size, skipping the download...\n");
            return 0;
        }
    }

    FILE *file = fopen(output_path, "wb");
    if (file == NULL)
        return 1;

    curl_easy_setopt(handle, CURLOPT_WRITEFUNCTION, write_function);
    curl_easy_setopt(handle, CURLOPT_URL, download_url);
    curl_easy_setopt(handle, CURLOPT_NOPROGRESS, 0L);
    curl_easy_setopt(handle, CURLOPT_XFERINFOFUNCTION, progress_func);
    curl_easy_setopt(handle, CURLOPT_PROGRESSDATA, progress);

    curl_easy_setopt(handle, CURLOPT_WRITEDATA, file);

    curl_easy_setopt(handle, CURLOPT_FAILONERROR, 1L);

    if (share != NULL)
        curl_easy_setopt(handle, CURLOPT_SHARE, share);

    CURLcode curlCode;
    int retryCount = 0;
    do {
        // Reset file position to start in case we have a partially-written file
        fseek(file, 0, SEEK_SET);
        curlCode = curl_easy_perform(handle);
        if ((curlCode == CURLE_OK) || cancelled || queueCancelled)
            break;
        ++retryCount;
        if (doRetrySleep)
            sleep(5);
    } while (retryCount < MAX_RETRIES);

    long httpCode = 0;
    curl_easy_getinfo(handle, CURLINFO_RESPONSE_CODE, &httpCode);

    if (httpCode != 200 || curlCode != CURLE_OK && curlCode != CURLE_WRITE_ERROR) {
        fclose(file);
        return 1;
    }

    fclose(file);
    return 0;
}

void setSelectedDir(const char *path) {
    if (selected_dir == NULL)
        selected_dir = (char *)malloc(strlen(path) + 1);
    if (strlen(path) > strlen(selected_dir)) {
        free(selected_dir);
        selected_dir = (char *)malloc(strlen(path) + 1);
    }
    strcpy(selected_dir, path);
}

char *getSelectedDir() {
    return selected_dir;
}

void setHideWiiVCWarning(bool value) {
    downloadWiiVC = value;
}

bool getHideWiiVCWarning() {
    return downloadWiiVC;
}

void setQueueCancelled(bool value) {
    queueCancelled = value;
}

void setCurrentTitle(const char *value) {
    if (currentTitle == NULL)
        currentTitle = (char *)malloc(strlen(value) + 1);
    if (strlen(value) > strlen(currentTitle)) {
        free(currentTitle);
        currentTitle = (char *)malloc(strlen(value) + 1);
    }
    strcpy(currentTitle, value);
    if(gameLabel != NULL)
        gtk_label_set_label(GTK_LABEL(gameLabel), currentTitle);
}

int downloadTitle(const char *titleID, const char *name, bool decrypt, bool deleteEncryptedContents) {
    // initialize some useful variables
    cancelled = false;
    if (queueCancelled) {
        return 0;
    }
    char *output_dir = (char *)malloc(1024);
    char *folder_name = (char *)malloc(1024);
    getTitleNameFromTid(strtoull(titleID, NULL, 16), folder_name);
    if ((selected_dir == NULL) || (strcmp(selected_dir, "") == 0) || !dirExists(selected_dir))
        selected_dir = show_folder_select_dialog();
    if ((selected_dir == NULL) || (strcmp(selected_dir, "") == 0) || !dirExists(selected_dir)) {
        free(folder_name);
        free(output_dir);
        return -1;
    }
    strcpy(output_dir, selected_dir);
    strcat(output_dir, "/");
    strcat(output_dir, folder_name);
    if (output_dir[strlen(output_dir) - 1] == '/' || output_dir[strlen(output_dir) - 1] == '\\') {
        output_dir[strlen(output_dir) - 1] = '\0';
    }
    char base_url[69];
    snprintf(base_url, 69, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", titleID);
    char download_url[81];
    char *output_path = (char *)malloc(strlen(output_dir) + 14);

// create the output directory if it doesn't exist
#ifdef _WIN32
    mkdir(output_dir);
#else
    mkdir(output_dir, 0777);
#endif

    // initialize curl
    curl_global_init(CURL_GLOBAL_ALL);

    // make an own handle for the tmd file, as we wanna download it to memory first
    CURL *tmd_handle = curl_easy_init();
    curl_easy_setopt(tmd_handle, CURLOPT_FAILONERROR, 1L);

    if (share == NULL) {
        share = curl_share_init();
        curl_share_setopt(share, CURLSHOPT_SHARE, CURL_LOCK_DATA_CONNECT);
        curl_share_setopt(share, CURLSHOPT_SHARE, CURL_LOCK_DATA_DNS);
    }

    if (share != NULL)
        curl_easy_setopt(tmd_handle, CURLOPT_SHARE, share);

    // Download the tmd and save it in memory, as we need some data from it
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEFUNCTION, WriteDataToMemory);
    snprintf(download_url, 73, "%s/%s", base_url, "tmd");
    curl_easy_setopt(tmd_handle, CURLOPT_URL, download_url);

    struct MemoryStruct tmd_mem;
    tmd_mem.memory = (uint8_t *)malloc(0);
    tmd_mem.size = 0;
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEDATA, (void *) &tmd_mem);
    CURLcode tmdCode = curl_easy_perform(tmd_handle);
    long httpCode = 0;
    curl_easy_getinfo(tmd_handle, CURLINFO_RESPONSE_CODE, &httpCode);

    if (httpCode != 200 || tmdCode != CURLE_OK) {
        showError("Error downloading title metadata.\nPlease check your internet connection\nOr your router might be blocking the NUS server");
        setQueueCancelled(true);
        cancelled = true;
    }
    curl_easy_cleanup(tmd_handle);
    // write out the tmd file
    sprintf(output_path, "%s/%s", output_dir, "title.cert");
    generateCert(output_path);
    sprintf(output_path, "%s/%s", output_dir, "title.tmd");
    FILE *tmd_file = fopen(output_path, "wb");
    if (!tmd_file) {
        log_error("The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        free(output_dir);
        free(output_path);
        free(folder_name);
        return -1;
    }
    fwrite(tmd_mem.memory, 1, tmd_mem.size, tmd_file);
    fclose(tmd_file);
    log_info("Finished downloading \"%s\".\n", output_path);

    TMD *tmd_data = (TMD *) tmd_mem.memory;

    uint16_t title_version = bswap_16(tmd_data->title_version);
    sprintf(output_path, "%s/%s", output_dir, "title.tik");
    char *titleKey = (char *)malloc(33);
    generateKey(titleID, titleKey);

    uint16_t content_count = bswap_16(tmd_data->num_contents);

    struct CURLProgress *progress = (struct CURLProgress *) malloc(sizeof(struct CURLProgress));
    memset(progress, 0, sizeof(struct CURLProgress));
    setCurrentTitle(name);
    handle = curl_easy_init();
    for (size_t i = 0; i < content_count; i++) {
        progress->titleSize += bswap_64(tmd_data->contents[i].size);
    }
    progress->totalSize = (char *)malloc(255);
    readable_fs(progress->titleSize, progress->totalSize);
    log_trace("Total size: %s (%zu)\n", progress->totalSize, progress->titleSize);
    sprintf(output_path, "%s/%s", output_dir, "title.tik");
    snprintf(download_url, 74, "%s/%s", base_url, "cetk");
    std::future<int> downloadFuture = std::async(downloadFile, download_url, output_path, progress, false);
    downloadFuture.wait();
    if (downloadFuture.get() != 0)
        generateTicket(output_path, strtoull(titleID, NULL, 16), titleKey, title_version);
    free(titleKey);
    uint8_t *tikData = (uint8_t *)malloc(2048);
    read_file(output_path, &tikData);
    int ret = 0;

    // Check the FST
    uint32_t fstID = bswap_32(tmd_data->contents[0].cid);
    sprintf(output_path, "%s/%08x.app", output_dir, fstID);
    snprintf(download_url, 78, "%s/%08x", base_url, fstID);
    progress->currentFile = (char *)malloc(255);
    sprintf(progress->currentFile, "%08x.app", fstID);
    if (downloadFile(download_url, output_path, progress, true) != 0) {
        showError("Error downloading FST\nPlease check your internet connection\nOr your router might be blocking the NUS server");
        cancelled = true;
        ret = -1;
    }
    uint8_t *decryptedFSTData = (uint8_t *)malloc(bswap_64(tmd_data->contents[0].size));
    TICKET *tik = (TICKET *) tikData;
    decryptFST(output_path, decryptedFSTData, tmd_data, tik->key);
    if (!validateFST(decryptedFSTData)) {
        showError("Error: Invalid FST Data, download is probably corrupt");
        log_error("Invalid FST Data, download is probably corrupt");
        cancelled = true;
        ret = -1;
    }
    if (!downloadWiiVC) {
        if (containsFile(decryptedFSTData, "fw.img")) {
            downloadWiiVC = ask("This is a Wii VC Title\nIt won't run on Cemu\nContinue downloading?");
            if (!downloadWiiVC)
                cancelled = true;
        }
    }
    free(decryptedFSTData);
    free(tikData);
    saveSettings(getSelectedDir(), getHideWiiVCWarning());

    for (int i = 0; i < content_count; i++) {
        if (!cancelled) {
            uint32_t id = bswap_32(tmd_data->contents[i].cid); // the id should usually be chronological, but we wanna be sure

            // add a curl handle for the content file (.app file)
            sprintf(output_path, "%s/%08x.app", output_dir, id);
            snprintf(download_url, 78, "%s/%08x", base_url, id);
            sprintf(progress->currentFile, "%08x.app", id);
            if (downloadFile(download_url, output_path, progress, true) != 0) {
                showError("Error downloading file\nPlease check your internet connection\nOr your router might be blocking the NUS server");
                cancelled = true;
                ret = -1;
                break;
            }

            if (bswap_16(tmd_data->contents[i].type) & TMD_CONTENT_TYPE_HASHED) {
                // add a curl handle for the hash file (.h3 file)
                sprintf(output_path, "%s/%08x.h3", output_dir, id);
                snprintf(download_url, 81, "%s/%08x.h3", base_url, id);
                sprintf(progress->currentFile, "%08x.h3", id);
                if (downloadFile(download_url, output_path, progress, true) != 0) {
                    showError("Error downloading file\nPlease check your internet connection\nOr your router might be blocking the NUS server");
                    cancelled = true;
                    ret = -1;
                    break;
                }
            }
        }
    }
    free(tmd_mem.memory);

    log_info("Downloading all files for TitleID %s done...\n", titleID);

    // cleanup curl stuff
    curl_easy_cleanup(handle);
    curl_global_cleanup();
    if (decrypt && !cancelled) {
        char *argv[2] = {"WiiUDownloader", _dirname(output_path)};
        if (cdecrypt(2, argv) != 0) {
            showError("Error: There was a problem decrypting the files.\nThe path specified for the download might be too long.\nPlease try downloading the files to a shorter path and try again.");
            ret = -2;
            goto out;
        }
    }
    if (deleteEncryptedContents)
        removeFiles(_dirname(output_path));
out:
    free(output_dir);
    free(output_path);
    free(folder_name);
    free(progress->totalSize);
    free(progress->currentFile);
    free(progress);
    if (share != NULL) {
        curl_share_cleanup(share);
        share = NULL;
    }
    return ret;
}
