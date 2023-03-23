#pragma once

#include <gtitles.h>
#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define TOKENPASTE(x, y)  x##y
#define TOKENPASTE2(x, y) TOKENPASTE(x, y)
#define PADDING(size)     char TOKENPASTE2(padding_, __LINE__)[size]

typedef struct __attribute__((__packed__)) {
    uint32_t sig_type;

    // Our header
    uint8_t magic_header[0x0A];
    PADDING(2);
    char app[0x10];
    char app_version[0x10];
    char file_type[0x10];
    PADDING(0xAF);
    uint8_t meta_version;
    uint8_t rand_area[0x50];
} NUS_HEADER;

typedef struct __attribute__((__packed__)) {
    NUS_HEADER header;

    // uint8_t sig[0x100];
    // PADDING(0x3C);
    char issuer[0x40];
    uint8_t ecdsa_pubkey[0x3c];
    uint8_t version;
    uint8_t ca_clr_version;
    uint8_t signer_crl_version;
    uint8_t key[0x10];
    PADDING(0x01);
    uint64_t ticket_id;
    uint32_t device_id;
    uint64_t tid;
    uint16_t sys_access;
    uint16_t title_version;
    PADDING(0x08);
    uint8_t license_type;
    uint8_t ckey_index;
    uint16_t property_mask;
    PADDING(0x28);
    uint32_t account_id;
    PADDING(0x01);
    uint8_t audit;
    PADDING(0x42);
    uint8_t limit_entries[0x40];
    uint16_t header_version; // we support version 1 only!
    uint16_t header_size;
    uint32_t total_hdr_size;
    uint32_t sect_hdr_offset;
    uint16_t num_sect_headers;
    uint16_t num_sect_header_entry_size;
    uint32_t header_flags;
} TICKET;

typedef struct __attribute__((__packed__)) {
    char issuer[0x40];
    PADDING(3);
    uint8_t version;
    char type[0x40];
    uint8_t sig[0x100];
    PADDING(4);
    uint32_t unknown_01;
    PADDING(0x34);
    uint32_t unknown_02;
    uint8_t cert[0x200];
    PADDING(0x3C);
} CA3_PPKI_CERT;

typedef struct __attribute__((__packed__)) {
    char issuer[0x40];
    PADDING(3);
    uint8_t version;
    char type[0x40];
    uint8_t sig[0x100];
    PADDING(4);
    uint32_t unknown_01;
    PADDING(0x34);
    uint32_t unknown_02;
    uint8_t cert[0x100];
    PADDING(0x3C);
} XSC_PPKI_CERT;

typedef struct __attribute__((__packed__)) {
    char issuer[0x40];
    PADDING(3);
    uint8_t version;
    char type[0x40];
    uint8_t sig[0x100];
    PADDING(4);
    uint32_t unknown_01;
    PADDING(0x34);
} CP8_PPKI_CERT;

typedef struct __attribute__((__packed__)) {
    NUS_HEADER header;

    CA3_PPKI_CERT cert1;
    XSC_PPKI_CERT cert2;
    CP8_PPKI_CERT cert3;
} CETK;

bool generateKey(const char *tid, char *out);
bool generateTicket(const char *path, uint64_t titleID, const char *titleKey, uint16_t titleVersion);
bool generateCert(const char *path);
void hex(uint64_t i, int digits, char *out);

#ifdef __cplusplus
}
#endif
