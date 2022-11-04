#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <inttypes.h>
#include <unistd.h>
#include <byteswap.h>
#ifndef _WIN32
    #include <sys/stat.h>
#endif

#include <keygen.h>
#include <ticket.h>
#include <curl/curl.h>

struct MemoryStruct {
  uint8_t* memory;
  size_t size;
};

struct PathFileStruct {
    char* file_path;
    FILE* file_pointer;
};

int progress_func(void* ptr, double TotalToDownload, double NowDownloaded, 
                    double TotalToUpload, double NowUploaded)
{
    // ensure that the file to be downloaded is not empty
    // because that would cause a division by zero error later on
    if (TotalToDownload <= 0.0) {
        return 0;
    }

    // how wide you want the progress meter to be
    int totaldotz=40;
    double fractiondownloaded = NowDownloaded / TotalToDownload;
    // part of the progressmeter that's already "full"
    int dotz = (int) round(fractiondownloaded * totaldotz);

    // create the "meter"
    int ii=0;
    printf("%3.0f%% [",fractiondownloaded*100);
    // part  that's full already
    for ( ; ii < dotz;ii++) {
        printf("=");
    }
    // remaining part (spaces)
    for ( ; ii < totaldotz;ii++) {
        printf(" ");
    }
    // and back to line begin - do not forget the fflush to avoid output buffering problems!
    printf("]\r");
    fflush(stdout);
    // if you don't return 0, the transfer will be aborted - see the documentation
    return 0; 
}

static size_t WriteDataToMemory(void* contents, size_t size, size_t nmemb, void* userp)
{
    size_t realsize = size * nmemb;
    struct MemoryStruct* mem = (struct MemoryStruct*) userp;

    mem->memory = realloc(mem->memory, mem->size + realsize);
    memcpy(&(mem->memory[mem->size]), contents, realsize);
    mem->size += realsize;

    return realsize;
}

void create_ticket(const char* title_id, const char* title_key, uint16_t title_version, const char* output_path)
{
    FILE* ticket_file = fopen(output_path, "wb");
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

static size_t write_data(void* data, size_t size, size_t nmemb, void* file_stream)
{
    size_t written = fwrite(data, size, nmemb, file_stream);
    return written;
}

int downloadFile(char *download_url, char *output_path) {
    CURL* new_handle = curl_easy_init();
    curl_easy_setopt(new_handle, CURLOPT_FAILONERROR, 1L);
    curl_easy_setopt(new_handle, CURLOPT_URL, download_url);
    curl_easy_setopt(new_handle, CURLOPT_WRITEFUNCTION, write_data);
    curl_easy_setopt(new_handle, CURLOPT_NOPROGRESS, 0);
    curl_easy_setopt(new_handle, CURLOPT_PROGRESSFUNCTION, progress_func); 

    FILE* output_file = fopen(output_path, "wb");
    if (!output_file) {
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }
    printf("Downloading file \"%s\".\n", download_url);

    struct PathFileStruct* struct_to_save = malloc(sizeof(struct PathFileStruct));
    struct_to_save->file_path = malloc(strlen(output_path) + 1);
    strcpy(struct_to_save->file_path, output_path);
    struct_to_save->file_pointer = output_file;
    curl_easy_setopt(new_handle, CURLOPT_WRITEDATA, output_file);
    curl_easy_setopt(new_handle, CURLOPT_PRIVATE, struct_to_save);
    curl_easy_perform(new_handle);
    curl_easy_cleanup(new_handle);
    return 0;
}

int downloadTitle(const char *titleID) {
    // initialize some useful variables
    char* output_dir = strdup(titleID);
    if (output_dir[strlen(output_dir)-1] == '/' || output_dir[strlen(output_dir)-1] == '\\') {
        output_dir[strlen(output_dir)-1] = '\0';
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
    CURL* tmd_handle = curl_easy_init();
    curl_easy_setopt(tmd_handle, CURLOPT_FAILONERROR, 1L);

    // Download the tmd and save it in memory, as we need some data from it
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEFUNCTION, WriteDataToMemory);
    snprintf(download_url, 73, "%s/%s", base_url, "tmd");
    curl_easy_setopt(tmd_handle, CURLOPT_URL, download_url);

    struct MemoryStruct tmd_data;
    tmd_data.memory = malloc(0);
    tmd_data.size = 0;
    curl_easy_setopt(tmd_handle, CURLOPT_WRITEDATA, (void*) &tmd_data);
    curl_easy_perform(tmd_handle);
    curl_easy_cleanup(tmd_handle);
    // write out the tmd file
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.cert");
    generateCert(output_path);
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tmd");
    FILE* tmd_file = fopen(output_path, "wb");
    if (!tmd_file) {
        free(output_dir);
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }
    write_data(tmd_data.memory, 1, tmd_data.size, tmd_file);
    fclose(tmd_file);
    printf("Finished downloading \"%s\".\n", output_path);

    uint16_t title_version;
    memcpy(&title_version, &tmd_data.memory[476], 2);
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tik");
    char titleKey[128];
    getTitleKeyFromTitleID(titleID, titleKey);
    create_ticket(titleID, titleKey, title_version, output_path);

    uint16_t content_count;
    memcpy(&content_count, &tmd_data.memory[478], 2);
    content_count = bswap_16(content_count);

    // Add all needed curl handles to the multi handle
    for (int i = 0; i < content_count; i++) {
        int offset = 2820 + (48 * i);
        uint32_t id; // the id should usually be chronological, but we wanna be sure
        memcpy(&id, &tmd_data.memory[offset], 4);
        id = bswap_32(id);

        // add a curl handle for the content file (.app file)
        snprintf(output_path, sizeof(output_path), "%s/%08X.app", output_dir, id);
        snprintf(download_url, 78, "%s/%08X", base_url, id);
        downloadFile(download_url, output_path);

        if ((tmd_data.memory[offset + 7] & 0x2) == 2) {
            // add a curl handle for the hash file (.h3 file)
            snprintf(output_path, sizeof(output_path), "%s/%08X.h3", output_dir, id);
            snprintf(download_url, 81, "%s/%08X.h3", base_url, id);
            downloadFile(download_url, output_path);
        }
    }
    free(tmd_data.memory);

    printf("Downloading all files for TitleID %s done...\n", titleID);

    // cleanup curl stuff
    curl_global_cleanup();
    free(output_dir);
}

int main(int argc, char** argv)
{
    if (argc != 2 && argc != 3) {
        printf("WiiUDownloader, (more or less) a C port of the FunKiiU program.\n");
        printf("It allows to download game files from the nintendo servers.\n\n");
        printf("Usage: ./WiiUDownloader <TitleID> [output directory]\n");
        exit(EXIT_SUCCESS);
    }
    if (strlen(argv[1]) != 16) {
        fprintf(stderr, "Error: TitleID has a wrong length!");
        exit(EXIT_FAILURE);
    }
    downloadTitle(argv[1]);
}
