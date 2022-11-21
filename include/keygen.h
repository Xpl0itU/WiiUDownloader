#pragma once

#include <gtitles.h>
#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Workaround weird macro concat ## behaviour
#define WUT_PP_CAT(a, b) WUT_PP_CAT_I(a, b)
#define WUT_PP_CAT_I(a, b) WUT_PP_CAT_II(~, a ## b)
#define WUT_PP_CAT_II(p, res) res

// Allow us to easily add UNKNOWN / PADDING bytes into our structs,
// generates unique variable names using __COUNTER__
#define WUT_UNKNOWN_BYTES(Size) char WUT_PP_CAT(__unk, __COUNTER__) [Size]

typedef struct {
    uint8_t *start;
    size_t size;
} TICKET_SECTION;

typedef struct __attribute__((__packed__)) {
    uint32_t unk01;
    uint32_t unk02;
    uint32_t unk03;
    uint32_t unk04;
    uint16_t unk05;
    WUT_UNKNOWN_BYTES(6);
    uint32_t unk06[8];
    WUT_UNKNOWN_BYTES(96);
} TICKET_HEADER_SECTION;

typedef struct __attribute__((__packed__)) {
    uint32_t sig_type;

    // Our header
    uint8_t magic_header[0x0A];
    WUT_UNKNOWN_BYTES(2);
    char app[0x10];
    char app_version[0x10];
    char file_type[0x10];
    WUT_UNKNOWN_BYTES(0xAF);
    uint8_t meta_version;
    uint8_t rand_area[0x50];
} NUS_HEADER;

typedef struct __attribute__((__packed__)) {
    NUS_HEADER header;

    // uint8_t sig[0x100];
    // WUT_UNKNOWN_BYTES(0x3C);
    char issuer[0x40];
    uint8_t ecdsa_pubkey[0x3c];
    uint8_t version;
    uint8_t ca_clr_version;
    uint8_t signer_crl_version;
    uint8_t key[0x10];
    WUT_UNKNOWN_BYTES(0x01);
    uint64_t ticket_id;
    uint32_t device_id;
    uint64_t tid;
    uint16_t sys_access;
    uint16_t title_version;
    WUT_UNKNOWN_BYTES(0x08);
    uint8_t license_type;
    uint8_t ckey_index;
    uint16_t property_mask;
    WUT_UNKNOWN_BYTES(0x28);
    uint32_t account_id;
    WUT_UNKNOWN_BYTES(0x01);
    uint8_t audit;
    WUT_UNKNOWN_BYTES(0x42);
    uint8_t limit_entries[0x40];
    uint16_t header_version; // we support version 1 only!
    uint16_t header_size;
    uint32_t total_hdr_size;
    uint32_t sect_hdr_offset;
    uint16_t num_sect_headers;
    uint16_t num_sect_header_entry_size;
    uint32_t header_flags;
} TICKET;

bool generateKey(const char *tid, char *out);
bool generateTicket(const char *path, uint64_t titleID, const char *titleKey, uint16_t titleVersion);
int char2int(char input);
void hex2bytes(const char *input, uint8_t *output);
void hex(uint64_t i, int digits, char *out);

#ifdef __cplusplus
}
#endif
