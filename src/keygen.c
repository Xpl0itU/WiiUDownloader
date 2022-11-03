#include <mbedtls/aes.h>
#include <mbedtls/md5.h>
#include <mbedtls/pkcs5.h>

#include <keygen.h>

#include <stdlib.h>
#include <string.h>

static const uint8_t KEYGEN_SECRET[10] = { 0xfd, 0x04, 0x01, 0x05, 0x06, 0x0b, 0x11, 0x1c, 0x2d, 0x49 };

static const uint8_t commonKey[16] = { 0xd7, 0xb0, 0x04, 0x02, 0x65, 0x9b, 0xa2, 0xab, 0xd2, 0xcb, 0x0d, 0xb2, 0x7f, 0xa2, 0xb6, 0x56 };

uint64_t convertStringToU64(const char *str)   // char * preferred
{
  char t[16] = "0123456789ABCDEF";

  uint64_t val = 0;
  for (int i = 0; i < strlen(str); i++)
  {
    val = val * 16;
    val = val + str[i] - '0';  // convert char to numeric value << this line needs to be expanded to include ABCDEF
  }
  return val;
}

int char2int(char input)
{
    if (input >= '0' && input <= '9')
        return input - '0';
    if (input >= 'A' && input <= 'F')
        return input - 'A' + 10;
    if (input >= 'a' && input <= 'f')
        return input - 'a' + 10;
    fprintf(stderr, "Error: Malformed input.\n");
    exit(1);
}

void hex2bytes(const char* input, uint8_t* output)
{
    int input_length = strlen(input);
    for (int i = 0; i < input_length; i += 2) {
        output[i / 2] = char2int(input[i]) * 16 + char2int(input[i + 1]);
    }
}

bool encryptAES(void *data, int data_len, const unsigned char *key, unsigned char *iv, void *encrypted)
{
    mbedtls_aes_context ctx;
    mbedtls_aes_init(&ctx);
    mbedtls_aes_setkey_enc(&ctx, key, 128);
    bool ret = mbedtls_aes_crypt_cbc(&ctx, MBEDTLS_AES_ENCRYPT, data_len, iv, data, encrypted) == 0;
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

static inline const char *transformPassword(TITLE_KEY in)
{
    switch(in)
    {
        case TITLE_KEY_mypass:
        case TITLE_KEY_MAGIC:
            return "mypass";
        case TITLE_KEY_nintendo:
            return "nintendo";
        case TITLE_KEY_test:
            return "test";
        case TITLE_KEY_1234567890:
            return "1234567890";
        case TITLE_KEY_Lucy131211:
            return "Lucy131211";
        case TITLE_KEY_fbf10:
            return "fbf10";
        case TITLE_KEY_5678:
            return "5678";
        case TITLE_KEY_1234:
            return "1234";
        case TITLE_KEY_:
            return "";
        default:
            return "mypass"; // Seems to work so far even for newest releases
    }
}

bool generateKey(const char *tid, TITLE_KEY title_key, uint8_t *out) {
    uint8_t *ti;
    hex2bytes(tid, ti);
    size_t i;
    size_t j;
    switch(getTidHighFromTid(convertStringToU64(tid)))
    {
        case TID_HIGH_VWII_IOS:
            ti += 2;
            i = 8 - 3;
            j = 8 - 3 + 10;
            break;
        default:
            i = 8 - 1;
            j = 8 - 1 + 10;
    }

    uint8_t key[17];
    // The salt is a md5 hash of the keygen secret + part of the title key
    memmove(key, KEYGEN_SECRET, 10);
    memmove(key + 10, ++ti, i);
    mbedtls_md5(key, j, key);

    // The key is the password salted with the md5 hash from above
    const char *pw = transformPassword(title_key);
    mbedtls_md_context_t ctx;
    mbedtls_md_setup(&ctx, mbedtls_md_info_from_type(MBEDTLS_MD_SHA1), 1);
    if(mbedtls_pkcs5_pbkdf2_hmac(&ctx, (const unsigned char *)pw, strlen(pw), key, 16, 20, 16, key) != 0)
        return false;

    // The final key needs to be AES encrypted with the Wii U common key and part of the title ID padded with zeroes as IV
    uint8_t iv[16];
    memmove(iv, tid, 8);
    memset(iv + 8, 0, 8);
    return encryptAES(key, 16, commonKey, iv, out);
}

void getTitleKeyFromTitleID(const char *tid, char *out) {
    uint8_t tKey[32];
    for(int i = 0; i <= getTitleEntriesSize(TITLE_CATEGORY_ALL); ++i) {
        if(getTitleEntries(TITLE_CATEGORY_ALL)[i].tid == convertStringToU64(tid)) {
            generateKey(tid, getTitleEntries(TITLE_CATEGORY_ALL)[i].key, tKey);
            sprintf(out, "%032x", tKey);
            break;
        }
    }
}