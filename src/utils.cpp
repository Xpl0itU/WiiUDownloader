#include <algorithm>
#include <map>
#include <math.h>
#include <vector>
#include <mbedtls/sha1.h>
#include <fstream>
#include <iostream>

#include <utils.h>

std::map<int, std::vector<unsigned char>> h0Hashes;
std::map<int, std::vector<unsigned char>> h1Hashes;
std::map<int, std::vector<unsigned char>> h2Hashes;
std::map<int, std::vector<unsigned char>> h3Hashes;
int blockCount = 0;

int get_chunk_from_stream(std::ifstream &in, std::vector<unsigned char> &buffer, std::vector<unsigned char> &overflowbuffer, int buffer_size) {
    int read = 0;
    buffer.resize(buffer_size);
    if (!overflowbuffer.empty()) {
        buffer.insert(buffer.begin(), overflowbuffer.begin(), overflowbuffer.end());
        overflowbuffer.resize(0);
    }
    in.read((char *) buffer.data() + read, buffer_size - read);
    read += in.gcount();
    if (read < buffer_size) {
        buffer.resize(read);
    }
    return read;
}

void setBlockCount(int count) {
    blockCount = count;
}

std::vector<unsigned char> generateHash(std::vector<unsigned char> content) {
    unsigned char hash[20];
    mbedtls_sha1(content.data(), content.size(), hash);
    return std::vector<unsigned char>(hash, hash + 20);
}

void CalculateH0Hashes(std::string file) {
    std::ifstream input(file, std::ios::binary);

    const int bufferSize = 0xFC00;

    std::vector<unsigned char> buffer(bufferSize);
    int total_blocks = (int)(input.tellg() / bufferSize) + 1;
    for (int block = 0; block < total_blocks; block++) {
        input.read((char*)&buffer[0], bufferSize);

        h0Hashes.emplace(block, generateHash(buffer));

        if (block % 100 == 0) {
            std::cout << "\rcalculating h0: " << 100 * block / total_blocks << "%";
        }
    }
    std::cout << "\rcalculating h0: done";
    setBlockCount(total_blocks);
}

void CalculateOtherHashes(int hashLevel, std::map<int, std::vector<unsigned char>> &inHashes, std::map<int, std::vector<unsigned char>> &outHashes) {
    int hash_level_pow = 1 << (4 * hashLevel);

    int hashesCount = (blockCount / hash_level_pow) + 1;
    for (int new_block = 0; new_block < hashesCount; new_block++) {
        std::vector<unsigned char> cur_hashes(16 * 20);
        for (int i = new_block * 16; i < (new_block * 16) + 16; i++) {
            if (inHashes.count(i) > 0)
                std::copy(inHashes[i].begin(), inHashes[i].end(), cur_hashes.begin() + ((i % 16) * 20));
        }
        outHashes.emplace(new_block, generateHash(cur_hashes));
    }
}

extern "C" void generateHashes(const char *path) {
    CalculateH0Hashes(path);
    CalculateOtherHashes(1, h0Hashes, h1Hashes);
    CalculateOtherHashes(2, h1Hashes, h2Hashes);
    CalculateOtherHashes(3, h2Hashes, h3Hashes);
}

int readFile(const char *fileName, std::vector<unsigned char> &destination) {
    std::ifstream file;
    file.open(fileName, std::ios::binary);
    if (!file.is_open()) {
        return 1;
    }

    file.seekg(0, file.end);
    int length = (int) file.tellg();
    file.seekg(0, file.beg);

    destination.resize(length);
    file.read((char *) &destination[0], length);

    file.close();
    return 0;
}

template<class T>
static bool compareVectors(std::vector<T> a, std::vector<T> b) {
    std::sort(a.begin(), a.end());
    std::sort(b.begin(), b.end());
    return (a == b);
}

extern "C" bool compareHashes(const char *h3hashPath) {
    std::vector<unsigned char> h3hash;
    readFile(h3hashPath, h3hash);
    bool ret = compareVectors(h3hash, h3Hashes.begin()->second);
    h0Hashes.clear();
    h1Hashes.clear();
    h2Hashes.clear();
    h3Hashes.clear();
    setBlockCount(0);
    return ret;
}