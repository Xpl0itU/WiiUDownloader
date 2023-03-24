#include <cstdio>
#include <cstring>
#include <filesystem>
#include <gtitles.h>
#include <log.h>
#include <titleInfo.h>
#include <utils.h>

#include <fstream>
#include <gtkmm.h>
#include <mbedtls/md.h>
#include <nfd.h>

GtkWindow *gameListWindow = nullptr;

static void normalizeFilename(const char *filename, char *out) {
    size_t j = 0;
    for (size_t i = 0; filename[i]; ++i) {
        char c = filename[i];
        if (c == '_') {
            if (j && out[j - 1] == '_') continue; // Don't allow consecutive underscores
            out[j++] = '_';
        } else if (c == ' ') {
            if (j && out[j - 1] == ' ') continue; // Don't allow consecutive spaces
            out[j++] = ' ';
        } else if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
            out[j++] = c;
        }
    }
    if (j > 0 && out[j - 1] == '_') j--; // Don't end with an underscore
    out[j] = '\0';
}

bool getTitleNameFromTid(uint64_t tid, char *out) {
    const TitleEntry *entries = getTitleEntries(TITLE_CATEGORY_ALL);
    const TitleEntry *found = std::find_if(entries, entries + getTitleEntriesSize(TITLE_CATEGORY_ALL), [&](const TitleEntry &e) { return e.tid == tid; });
    if (found != entries + getTitleEntriesSize(TITLE_CATEGORY_ALL)) {
        char *name = (char *) malloc(1024);
        normalizeFilename(found->name, name);
        sprintf(out, "%s [%s] [%016llx]", name, getFormattedKind(tid), found->tid);
        free(name);
        return true;
    }
    sprintf(out, "Unknown");
    return false;
}

bool getUpdateFromBaseGame(uint64_t titleID, uint64_t *out) {
    if (!isGame(titleID))
        return false;
    uint64_t updateTID = titleID | 0x0000000E00000000;
    char *name = (char *) malloc(1024);
    if (getTitleNameFromTid(updateTID, name)) {
        *out = updateTID;
        free(name);
        return true;
    }
    free(name);
    return false;
}

void showError(const char *text) {
    Gtk::MessageDialog dlg(text, false, Gtk::MESSAGE_ERROR, Gtk::BUTTONS_OK);
    log_error(text);
    dlg.run();
}

bool ask(const char *question) {
    Gtk::MessageDialog dlg(question, true, Gtk::MESSAGE_QUESTION, Gtk::BUTTONS_YES_NO, true);
    return dlg.run() == Gtk::RESPONSE_YES;
}

char *dirname(char *path) {
    int len = strlen(path);
    int last = len - 1;
    char *parent = (char *) malloc(sizeof(char) * (len + 1));
    strcpy(parent, path);
    parent[len] = '\0';

    while (last >= 0) {
        if (parent[last] == '/') {
            parent[last] = '\0';
            break;
        }
        last--;
    }
    return parent;
}

char *show_folder_select_dialog() {
    NFD_Init();

    nfdchar_t *outPath = nullptr;

    NFD_PickFolder(&outPath, nullptr);

    // Quit NFD
    NFD_Quit();

    return outPath;
}

bool isThisDecryptedFile(const char *path) {
    if (std::string(path).find("code") != std::string::npos ||
        std::string(path).find("content") != std::string::npos ||
        std::string(path).find("meta") != std::string::npos) {
        return true;
    }
    return false;
}

void removeFiles(const char *path) {
    for (const auto &entry : std::filesystem::directory_iterator(path)) {
        if (entry.is_regular_file() && !isThisDecryptedFile(reinterpret_cast<const char *>(entry.path().c_str()))) {
            std::filesystem::remove(entry.path());
        }
    }
}

bool fileExists(const char *filename) {
    std::ifstream file(filename);
    return file.good();
}

void setGameList(GtkWindow *window) {
    gameListWindow = window;
}

void minimizeGameListWindow() {
    gtk_window_iconify(gameListWindow);
}

int compareHash(const char *input, const char *expectedHash) {
    unsigned char output[32];
    mbedtls_md_context_t ctx;
    const mbedtls_md_info_t *info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);

    mbedtls_md_init(&ctx);
    mbedtls_md_setup(&ctx, info, 0);
    mbedtls_md_starts(&ctx);
    mbedtls_md_update(&ctx, (const unsigned char *) input, strlen(input));
    mbedtls_md_finish(&ctx, output);
    mbedtls_md_free(&ctx);

    char outputHex[2 * sizeof(output) + 1];
    for (int i = 0; i < sizeof(output); ++i) {
        sprintf(outputHex + 2 * i, "%02x", output[i]);
    }

    return strcmp(expectedHash, outputHex);
}

size_t getFilesizeFromFile(FILE *file) {
    fseek(file, 0L, SEEK_END);
    size_t fSize = ftell(file);
    fseek(file, 0L, SEEK_SET);
    return fSize;
}