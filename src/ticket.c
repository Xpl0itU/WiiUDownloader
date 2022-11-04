#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ticket.h>
#include <keygen.h>

void writeVoidBytes(FILE* fp, uint32_t len)
{
	uint8_t bytes[len];
	memset(bytes, 0, len);
	fwrite(bytes, 1, len, fp);
}

void writeCustomBytes(FILE *fp, const char *str)
{
	if(str[0] == '0' && str[1] == 'x')
		str += 2;
	
	size_t size = strlen(str) >> 1;
	uint8_t bytes[size];
	hex2bytes(str, bytes);
	fwrite(bytes, 1, size, fp);
}

void writeRandomBytes(FILE* fp, uint32_t len) {
	uint8_t bytes[len];
	for (uint32_t i = 0; i < len; i++) {
		bytes[i] = rand() % 0xFF;
	}
	fwrite(bytes, len, 1, fp);
}

static void writeHeader(FILE *fp) {
	writeCustomBytes(fp, "0x00010004000102030405060708090000"); // Magic 32 bit value + our magic value + padding
	writeCustomBytes(fp, "0x4E555373706C69"); // "NUSspli"
	writeVoidBytes(fp, 0x9);
	int vl = strlen("TEST");
	fwrite("TEST", 1, vl, fp);
	
	writeVoidBytes(fp, 0x10 - vl);
	char *cb;
	int v;
    cb = "0x4365727469666963617465"; // "Certificate"
    v = 0xB4;
	
	writeCustomBytes(fp, cb);
	writeVoidBytes(fp, v);
	writeCustomBytes(fp, "0x01"); // TODO: Don't hardcode in here
	writeRandomBytes(fp, 0x14);
	writeVoidBytes(fp, 0x3C);
}

bool generateCert(const char *path) {
	FILE *cert = fopen(path, "wb");
	if(cert == NULL)
		return false;

	// NUSspli adds its own header.
	writeHeader(cert);

	// Some SSH certificate
	writeCustomBytes(cert, "0x526F6F742D43413030303030303033"); // "Root-CA00000003"
	writeVoidBytes(cert, 0x34);
	writeCustomBytes(cert, "0x0143503030303030303062"); // "?CP0000000b"
	writeVoidBytes(cert, 0x36);
	writeRandomBytes(cert, 0x104);
	writeCustomBytes(cert, "0x00010001");
	writeVoidBytes(cert, 0x34);
	writeCustomBytes(cert, "0x00010003");
	writeRandomBytes(cert, 0x200);
	writeVoidBytes(cert, 0x3C);

	// Next certificate
	writeCustomBytes(cert, "0x526F6F74"); // "Root"
	writeVoidBytes(cert, 0x3F);
	writeCustomBytes(cert, "0x0143413030303030303033"); // "?CA00000003"
	writeVoidBytes(cert, 0x36);
	writeRandomBytes(cert, 0x104);
	writeCustomBytes(cert, "0x00010001");
	writeVoidBytes(cert, 0x34);
	writeCustomBytes(cert, "0x00010004");
	writeRandomBytes(cert, 0x100);
	writeVoidBytes(cert, 0x3C);

	// Last certificate
	writeCustomBytes(cert, "0x526F6F742D43413030303030303033"); // "Root-CA00000003"
	writeVoidBytes(cert, 0x34);
	writeCustomBytes(cert, "0x0158533030303030303063"); // "?XS0000000c"
	writeVoidBytes(cert, 0x36);
	writeRandomBytes(cert, 0x104);
	writeCustomBytes(cert, "0x00010001");
	writeVoidBytes(cert, 0x34);

    fwrite(NULL, 0, 0, cert);

	return true;
}