#include <mbedtls/aes.h>
#include <mbedtls/md5.h>
#include <mbedtls/pkcs5.h>

#include <keygen.h>
#include <titleInfo.h>
#include <utils.h>
#include <version.h>

#include <cstdlib>
#include <cstring>

#define KEYGEN_SECRET "fd040105060b111c2d49"

static const uint8_t keygen_pw[] = {0x6d, 0x79, 0x70, 0x61, 0x73, 0x73};
static const uint8_t commonKey[16] = {0xd7, 0xb0, 0x04, 0x02, 0x65, 0x9b, 0xa2, 0xab, 0xd2, 0xcb, 0x0d, 0xb2, 0x7f, 0xa2, 0xb6, 0x56};

static const uint8_t magic_header[10] = {0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09};

static void rndBytes(char *out, size_t size) {
    while (--size) {
        *out++ = rand() % 256;
    }
}

static void generateHeader(bool isTicket, NUS_HEADER *out) {
    memmove(out->magic_header, magic_header, 10);
    memmove(out->app, "WiiUDownloader", strlen("WiiUDownloader"));
    memmove(out->app_version, "v" VERSION, strlen("v" VERSION));

    if (isTicket)
        memmove(out->file_type, "Ticket", strlen("Ticket"));
    else
        memmove(out->file_type, "Certificate", strlen("Certificate"));

    out->sig_type = bswap_32(0x00010004);
    out->meta_version = 0x01;
    rndBytes((char *) out->rand_area, sizeof(out->rand_area));
}

static int char2int(char input) {
    if (input >= '0' && input <= '9')
        return input - '0';
    if (input >= 'A' && input <= 'F')
        return input - 'A' + 10;
    if (input >= 'a' && input <= 'f')
        return input - 'a' + 10;
    fprintf(stderr, "Error: Malformed input: %c\n", input);
    exit(1);
}

static void hex2bytes(const char *input, uint8_t *output) {
    int input_length = strlen(input);
    for (int i = 0; i < input_length; i += 2) {
        output[i / 2] = char2int(input[i]) * 16 + char2int(input[i + 1]);
    }
}

static bool encryptAES(void *data, int data_len, const unsigned char *key, unsigned char *iv, void *encrypted) {
    mbedtls_aes_context ctx;
    mbedtls_aes_init(&ctx);
    mbedtls_aes_setkey_enc(&ctx, key, 128);
    bool ret = mbedtls_aes_crypt_cbc(&ctx, MBEDTLS_AES_ENCRYPT, data_len, iv, (const unsigned char *) data, (unsigned char *) encrypted) == 0;
    /*
     * We're not calling mbedtls_aes_free() as at the time of writing
     * all it does is overwriting the mbedtls_aes_context struct with
     * zeros.
     * As game key calculation isn't top secret we don't need this.
     *
     * TODO: Check the codes at every mbed TLS update to make sure
     * we won't need to call it.
     */
    // mbedtls_aes_free(&ctx);
    return ret;
}

void hex(uint64_t i, int digits, char *out) {
    char x[8]; // max 99 digits!
    sprintf(x, "%%0%illx", digits);
    sprintf(out, x, i);
}

bool generateKey(const char *tid, char *out) {
    char *ret = (char *) malloc(33);
    if (ret == nullptr)
        return false;

    char *tmp = const_cast<char *>(tid);
    while (tmp[0] == '0' && tmp[1] == '0')
        tmp += 2;

    char *h = (char *) malloc(1024);
    strcpy(h, KEYGEN_SECRET);
    strcat(h, tmp);

    size_t bhl = strlen(h) >> 1;
    uint8_t bh[bhl];
    for (size_t i = 0, j = 0; j < bhl; i += 2, j++)
        bh[j] = (h[i] % 32 + 9) % 25 * 16 + (h[i + 1] % 32 + 9) % 25;
    free(h);

    uint8_t md5sum[16];
    mbedtls_md5(bh, bhl, md5sum);

    uint8_t key[16];
    mbedtls_md_context_t ctx;
    mbedtls_md_setup(&ctx, mbedtls_md_info_from_type(MBEDTLS_MD_SHA1), 1);
    if (mbedtls_pkcs5_pbkdf2_hmac(&ctx, (const unsigned char *) keygen_pw, sizeof(keygen_pw), md5sum, 16, 20, 16, key) != 0)
        return false;

    uint8_t iv[16];
    for (size_t i = 0, j = 0; j < 8; i += 2, j++)
        iv[j] = (tid[i] % 32 + 9) % 25 * 16 + (tid[i + 1] % 32 + 9) % 25;

    memset(&iv[8], 0, 8);
    encryptAES(key, 16, commonKey, iv, key);

    tmp = ret;
    for (int i = 0; i < 16; i++, tmp += 2)
        sprintf(tmp, "%02x", key[i]);

    sprintf(out, "%s", ret);

    return true;
}

bool generateTicket(const char *path, uint64_t titleID, const char *titleKey, uint16_t titleVersion) {
    TICKET ticket;
    memset(&ticket, 0x00, sizeof(TICKET));

    hex2bytes(titleKey, ticket.key);

    generateHeader(true, &ticket.header);
    rndBytes((char *) &ticket.ecdsa_pubkey, sizeof(ticket.ecdsa_pubkey));
    rndBytes((char *) &ticket.ticket_id, sizeof(uint64_t));
    ticket.ticket_id &= 0x0000FFFFFFFFFFFF;
    ticket.ticket_id |= 0x0005000000000000;
    ticket.ticket_id = bswap_64(ticket.ticket_id);

    memmove(ticket.issuer, "Root-CA00000003-XS0000000c", strlen("Root-CA00000003-XS0000000c"));

    ticket.version = 0x01;
    ticket.tid = bswap_64(titleID);
    ticket.title_version = bswap_16(titleVersion);
    ticket.property_mask = bswap_16(0xFFFF);

    // We support zero sections only
    ticket.header_version = bswap_16(0x0001);
    if (!isDLC(ticket.tid))
        ticket.total_hdr_size = bswap_32(0x00000014);
    else {
        ticket.total_hdr_size = bswap_32(0x000000AC);
        ticket.sect_hdr_offset = bswap_32(0x00000014);
        ticket.num_sect_headers = bswap_16(0x0001);
        ticket.num_sect_header_entry_size = bswap_16(0x0014);
    }

    FILE *tik = fopen(path, "wb");
    if (tik == nullptr)
        return false;

    fwrite(&ticket, 1, sizeof(TICKET), tik);

    if (isDLC(titleID)) {
        TICKET_HEADER_SECTION section;
        memset(&section, 0x00, sizeof(TICKET_HEADER_SECTION));

        section.unk01 = bswap_32(0x00000028);
        section.unk02 = bswap_32(0x00000001);
        section.unk03 = bswap_32(0x00000084);
        section.unk04 = bswap_32(0x00000084);
        section.unk05 = bswap_16(0x0003);
        for (int i = 0; i < 8; i++)
            section.unk06[i] = 0xFFFFFFFF;

        fwrite(&section, 1, sizeof(TICKET_HEADER_SECTION), tik);
    }

    fclose(tik);
    return true;
}

bool generateCert(const char *path) {
    CETK cetk;
    memset(&cetk, 0x00, sizeof(CETK));

    generateHeader(false, &cetk.header);

    memmove(cetk.cert1.issuer, "Root-CA00000003", strlen("Root-CA00000003"));
    memmove(cetk.cert1.type, "CP0000000b", strlen("CP0000000b"));

    memmove(cetk.cert2.issuer, "Root", strlen("Root"));
    memmove(cetk.cert2.type, "CA00000003", strlen("CA00000003"));

    memmove(cetk.cert3.issuer, "Root-CA00000003", strlen("Root-CA00000003"));
    memmove(cetk.cert3.type, "XS0000000c", strlen("XS0000000c"));

    rndBytes((char *) &cetk.cert1.sig, sizeof(cetk.cert1.sig));
    rndBytes((char *) &cetk.cert1.cert, sizeof(cetk.cert1.cert));
    rndBytes((char *) &cetk.cert2.sig, sizeof(cetk.cert2.sig));
    rndBytes((char *) &cetk.cert2.cert, sizeof(cetk.cert2.cert));
    rndBytes((char *) &cetk.cert3.sig, sizeof(cetk.cert3.sig));

    cetk.cert1.version = 0x01;
    cetk.cert1.unknown_01 = bswap_32(0x00010001);
    cetk.cert1.unknown_02 = bswap_32(0x00010003);

    cetk.cert2.version = 0x01;
    cetk.cert2.unknown_01 = bswap_32(0x00010001);
    cetk.cert2.unknown_02 = bswap_32(0x00010004);

    cetk.cert3.version = 0x01;
    cetk.cert3.unknown_01 = bswap_32(0x00010001);

    FILE *cert = fopen(path, "wb");
    if (cert == nullptr)
        return false;

    fwrite(&cetk, 1, sizeof(CETK), cert);
    fclose(cert);
    return true;
}
