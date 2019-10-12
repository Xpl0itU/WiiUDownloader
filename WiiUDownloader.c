#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <inttypes.h>
#include <unistd.h>
#include <byteswap.h>

#include <curl/curl.h>


struct MemoryStruct {
  uint8_t* memory;
  size_t size;
};

struct PathFileStruct {
    char* file_path;
    FILE* file_pointer;
};


static size_t WriteDataToMemory(void* contents, size_t size, size_t nmemb, void* userp)
{
    size_t realsize = size * nmemb;
    struct MemoryStruct* mem = (struct MemoryStruct*) userp;

    mem->memory = realloc(mem->memory, mem->size + realsize);
    memcpy(&(mem->memory[mem->size]), contents, realsize);
    mem->size += realsize;

    return realsize;
}


void create_ticket(char* title_id, char* title_key, uint16_t title_version, char* output_path)
{
    FILE* ticket_file = fopen(output_path, "wb");
    if (!ticket_file) {
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }

    uint8_t ticket_data[848] = "\x00\x01\x00\x04\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\xd1\x5e\xa5\xed\x15\xab\xe1\x1a\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x52\x6f\x6f\x74\x2d\x43\x41\x30\x30\x30\x30\x30\x30\x30\x33\x2d\x58\x53\x30\x30\x30\x30\x30\x30\x30\x63\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\xfe\xed\xfa\xce\x01\x00\x00\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\xcc\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xaa\xaa\xaa\xaa\xaa\xaa\xaa\xaa\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x14\x00\x00\x00\xac\x00\x00\x00\x14\x00\x01\x00\x14\x00\x00\x00\x00\x00\x00\x00\x28\x00\x00\x00\x01\x00\x00\x00\x84\x00\x00\x00\x84\x00\x03\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00";
    char* pos;
    pos = title_id;
    for (int i = 0; i < 8; i++) {
        sscanf(pos, "%2hhx", &ticket_data[476 + i]);
        pos += 2;
    }
    pos = title_key;
    for (int i = 0; i < 16; i++) {
        sscanf(pos, "%2hhx", &ticket_data[447 + i]);
        pos += 2;
    }
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


void add_curl_handle(CURLM* curl_multi_handle, char* download_url, char* output_path)
{
    CURL* new_handle = curl_easy_init();
    curl_easy_setopt(new_handle, CURLOPT_FAILONERROR, 1L);
    curl_easy_setopt(new_handle, CURLOPT_URL, download_url);
    curl_easy_setopt(new_handle, CURLOPT_WRITEFUNCTION, write_data);

    FILE* output_file = fopen(output_path, "wb");
    if (!output_file) {
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }
    printf("Queuing up download for file \"%s\".\n", download_url);

    struct PathFileStruct* struct_to_save = malloc(sizeof(struct PathFileStruct));
    struct_to_save->file_path = malloc(strlen(output_path) + 1);
    strcpy(struct_to_save->file_path, output_path);
    struct_to_save->file_pointer = output_file;
    curl_easy_setopt(new_handle, CURLOPT_WRITEDATA, output_file);
    curl_easy_setopt(new_handle, CURLOPT_PRIVATE, struct_to_save);
    curl_multi_add_handle(curl_multi_handle, new_handle);
}


int main(int argc, char** argv)
{
    if (argc != 3 && argc != 4) {
        printf("WiiUDownloader, (more or less) a C port of the FunKiiU program.\n");
        printf("It allows to download game files from the nintendo servers.\n\n");
        printf("Usage: ./WiiUDownloader <TitleID> <TitleKey> [output directory]\n");
        exit(EXIT_SUCCESS);
    }
    if (strlen(argv[1]) != 16) {
        fprintf(stderr, "Error: TitleID has a wrong length!");
        exit(EXIT_FAILURE);
    }
    if (strlen(argv[2]) != 32) {
        fprintf(stderr, "Error: TitleKey has a wrong length!");
        exit(EXIT_FAILURE);
    }

    // initialize some useful variables
    char* output_dir = (argc == 4) ? argv[3] : argv[1];
    if (output_dir[strlen(output_dir)-1] == '/' || output_dir[strlen(output_dir)-1] == '\\') {
        output_dir[strlen(output_dir)-1] = '\0';
    }
    char base_url[69];
    snprintf(base_url, 69, "http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", argv[1]);
    char download_url[81];
    char output_path[strlen(output_dir) + 14];

    // create the output directory if it doesn't exist
    #if defined(_WIN32)
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
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tmd");
    FILE* tmd_file = fopen(output_path, "wb");
    if (!tmd_file) {
        fprintf(stderr, "Error: The file \"%s\" couldn't be opened. Will exit now.\n", output_path);
        exit(EXIT_FAILURE);
    }
    write_data(tmd_data.memory, 1, tmd_data.size, tmd_file);
    fclose(tmd_file);
    printf("Finished downloading \"%s\".\n", output_path);

    uint16_t title_version;
    memcpy(&title_version, &tmd_data.memory[476], 2);
    snprintf(output_path, sizeof(output_path), "%s/%s", output_dir, "title.tik");
    create_ticket(argv[1], argv[2], title_version, output_path);

    uint16_t content_count;
    memcpy(&content_count, &tmd_data.memory[478], 2);
    content_count = bswap_16(content_count);

    // Download content asynchronously using a multi handle
    CURLM* curl_multi_handle = curl_multi_init();
    // Add all needed curl handles to the multi handle
    for (int i = 0; i < content_count; i++) {
        int offset = 2820 + (48 * i);
        uint32_t id; // the id should usually be chronological, but we wanna be sure
        memcpy(&id, &tmd_data.memory[offset], 4);
        id = bswap_32(id);

        // add a curl handle for the content file (.app file)
        snprintf(output_path, sizeof(output_path), "%s/%08X.app", output_dir, id);
        snprintf(download_url, 78, "%s/%08X", base_url, id);
        add_curl_handle(curl_multi_handle, download_url, output_path);

        if ((tmd_data.memory[offset + 7] & 0x2) == 2) {
            // add a curl handle for the hash file (.h3 file)
            snprintf(output_path, sizeof(output_path), "%s/%08X.h3", output_dir, id);
            snprintf(download_url, 81, "%s/%08X.h3", base_url, id);
            add_curl_handle(curl_multi_handle, download_url, output_path);
        }
    }
    free(tmd_data.memory);

    printf("Downloading all files for TitleID %s asynchronously...\n", argv[1]);

    // Perform the download requests
    int still_alive = 1;
    while (still_alive) {
        curl_multi_perform(curl_multi_handle, &still_alive);

        CURLMsg* msg;
        int msgs_left = 1;
        while ((msg = curl_multi_info_read(curl_multi_handle, &msgs_left))) {
            if (msg->msg == CURLMSG_DONE) {
                CURL* finished_handle = msg->easy_handle;
                struct PathFileStruct* saved_struct;
                curl_easy_getinfo(finished_handle, CURLINFO_PRIVATE, &saved_struct);
                printf("Finished downloading \"%s\".\n", saved_struct->file_path);
                free(saved_struct->file_path);
                fclose(saved_struct->file_pointer);
                free(saved_struct);
                curl_multi_remove_handle(curl_multi_handle, finished_handle);
                curl_easy_cleanup(finished_handle);
            } else {
                fprintf(stderr, "AN ERROR OCCURED!\n");
            }
        }

        if (still_alive) {
            curl_multi_wait(curl_multi_handle, NULL, 0, 1000, NULL);
        }
    }

    // cleanup curl stuff
    curl_multi_cleanup(curl_multi_handle);
    curl_global_cleanup();
}
