#include <fst.h>

#include <stdio.h>

#include <mbedtls/aes.h>
#include <utils.h>

static const uint8_t commonKey[16] = {0xD7, 0xB0, 0x04, 0x02, 0x65, 0x9B, 0xA2, 0xAB, 0xD2, 0xCB, 0x0D, 0xB2, 0x7F, 0xA2, 0xB6, 0x56};

static __inline uint32_t getbe32(const void *p) {
    return bswap_32(*(const uint32_t *) (const uint8_t *) (p));
}

bool decryptFST(const char *path, uint8_t *outputBuffer, TMD *tmd, uint8_t *titleKey) {
    const uint64_t contentSize = bswap_64(tmd->contents[0].size);
    uint8_t *buffer = (uint8_t *) malloc(contentSize);
    if (buffer == NULL) {
        fprintf(stderr, "Couldn't allocate buffer for reading the encrypted FST");
        return false;
    }
    if (loadFile(path, &buffer) != contentSize) {
        fprintf(stderr, "FST size mismatch between the TMD and the filesize, file might me corrupted.");
        free(buffer);
        return false;
    }

    uint8_t titleID[16];
    memset(titleID, 0, 16);
    for (int i = 0; i < sizeof(uint64_t); ++i) {
        titleID[i] = (tmd->tid >> (8 * i)) & 0xff;
    }

    mbedtls_aes_context aes;
    mbedtls_aes_init(&aes);
    mbedtls_aes_setkey_dec(&aes, commonKey, sizeof(commonKey) * 8);
    mbedtls_aes_crypt_cbc(&aes, MBEDTLS_AES_DECRYPT, 16, titleID, titleKey, titleKey);
    mbedtls_aes_setkey_dec(&aes, titleKey, 128);

    uint8_t iv[16];
    memset(iv, 0, 16);

    mbedtls_aes_crypt_cbc(&aes, MBEDTLS_AES_DECRYPT, contentSize, iv, buffer, outputBuffer);
    free(buffer);
    return true;
}

bool validateFST(uint8_t *data) {
    struct FST *fst = (struct FST *) data;

    if (bswap_32(fst->MagicBytes) != 0x46535400) {
        return false;
    }
    return true;
}

bool containsFile(uint8_t *data, const char *path) {
    struct FST *fst = (struct FST *) data;
    struct FEntry *fe = (struct FEntry *) (data + 0x20 + (uintptr_t) getbe32(&fst->EntryCount) * 0x20);

    uint32_t entries = getbe32(data + 0x20 + (uintptr_t) getbe32(&fst->EntryCount) * 0x20 + 8);
    uint32_t name_offset = 0x20 + getbe32(&fst->EntryCount) * 0x20 + entries * 0x10;

    if (bswap_32(fst->MagicBytes) != 0x46535400) {
        return false;
    }

    for (uint32_t i = 1; i < entries; ++i) {
        uint32_t offset = getbe32(&fe[i].TypeName) & 0x00FFFFFF;
        if (strcmp(path, data + name_offset + offset) == 0)
            return true;
    }
    return false;
}