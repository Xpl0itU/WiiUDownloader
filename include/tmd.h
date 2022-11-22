#pragma once

#include <keygen.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C"
{
#endif

    // From: https://wiiubrew.org/wiki/Title_metadata
    // And: https://github.com/Maschell/nuspacker

#define TMD_CONTENT_TYPE_ENCRYPTED 0x0001
#define TMD_CONTENT_TYPE_HASHED    0x0002 // Never seen alone, alsways combined with TMD_CONTENT_TYPE_ENCRYPTED
#define TMD_CONTENT_TYPE_CONTENT   0x2000
#define TMD_CONTENT_TYPE_UNKNOWN   0x4000 // Never seen alone, alsways combined with TMD_CONTENT_TYPE_CONTENT

    typedef struct __attribute__((__packed__)) {
        uint32_t cid;
        uint16_t index;
        uint16_t type;
        uint64_t size;
        uint32_t hash[8];
    } TMD_CONTENT;

    typedef struct __attribute__((__packed__)) {
        uint16_t index;
        uint16_t count;
        uint32_t hash[8];
    } TMD_CONTENT_INFO;

    typedef struct __attribute__((__packed__)) {
        uint32_t sig_type;
        uint8_t sig[256];
        PADDING(60);
        uint8_t issuer[64];
        uint8_t version;
        uint8_t ca_crl_version;
        uint8_t signer_crl_version;
        PADDING(1);
        uint64_t sys_version;
        uint64_t tid;
        uint32_t type;
        uint16_t group;
        PADDING(62);
        uint32_t access_rights;
        uint16_t title_version;
        uint16_t num_contents;
        uint16_t boot_index;
        PADDING(2);
        uint32_t hash[8];
        TMD_CONTENT_INFO content_infos[64];
        TMD_CONTENT contents[0];
    } TMD;

#ifdef __cplusplus
}
#endif