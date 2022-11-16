#include <algorithm>
#include <cstddef>
#include <fstream>
#include <iostream>
#include <mbedtls/sha1.h>
#include <string.h>
#include <unordered_map>
#include <vector>

#include <utils.h>

int blockCount = 0;

// using byte as a vector of unsigned char
std::vector<unsigned char> buffer;
std::vector<unsigned char> overflowbuffer; // overflow buffer
int read = 0;
int block = 0;
int total_blocks = 0;

std::unordered_map<int, std::vector<unsigned char>> h0hashes;
std::unordered_map<int, std::vector<unsigned char>> h1hashes;
std::unordered_map<int, std::vector<unsigned char>> h2hashes;
std::unordered_map<int, std::vector<unsigned char>> h3hashes;

// Get a chunk from a stream, using an input stream, a buffer, and an overflow buffer
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

void calculate_h0_hashes(const char *file) {
    // open file
    std::ifstream in(file, std::ios::binary);
    if (!in) {
        throw;
    }

    int buffer_size = 0xFC00;
    buffer.resize(buffer_size);
    overflowbuffer.resize(buffer_size);

    // read chunks from file into buffer
    do {
        read = get_chunk_from_stream(in, buffer, overflowbuffer, buffer_size);
        if (read != buffer_size) {
            // if not all data could be read from the file:
            // resize buffer accordingly
            buffer.resize(read);
        }
        // calculate hash of current buffer
        unsigned char hash[20];
        std::vector<unsigned char> hashVector;
        unsigned char *data = buffer.data();
        mbedtls_sha1(data, 20, hash);
        std::copy(hash, hash + 20, std::back_inserter(hashVector));
        h0hashes.emplace(block++, hashVector);
    } while (read == buffer_size);
    blockCount = block;
}

void CalculateOtherHashes(int hashLevel, std::unordered_map<int, std::vector<unsigned char>> &inHashes, std::unordered_map<int, std::vector<unsigned char>> &outHashes) {
    int hash_level_pow = 1 << (4 * hashLevel);

    int hashesCount = (blockCount / hash_level_pow) + 1;
    for (int new_block = 0; new_block < hashesCount; new_block++) {
        std::vector<unsigned char> cur_hashes(16 * 20);
        for (int i = new_block * 16; i < (new_block * 16) + 16; i++) {
            if (inHashes.count(i) > 0)
                std::copy(inHashes[i].begin(), inHashes[i].end(), cur_hashes.begin() + ((i % 16) * 20));
        }
        unsigned char hash[20];
        std::vector<unsigned char> hashVector;
        unsigned char *data = cur_hashes.data();
        mbedtls_sha1(data, 20, hash);
        std::copy(hash, hash + 20, std::back_inserter(hashVector));
        outHashes.emplace(new_block, hashVector);
    }
}

extern "C" void generateHashes(const char *path) {
    calculate_h0_hashes(path);
    CalculateOtherHashes(1, h0hashes, h1hashes);
    CalculateOtherHashes(2, h1hashes, h2hashes);
    CalculateOtherHashes(3, h2hashes, h3hashes);
}

int readFile(const char *fileName, std::vector<unsigned char> &destination) {
    std::ifstream file(fileName, std::ios::binary);
    if (!file.is_open())
        return -1;

    file.seekg(0, file.end);
    int length = file.tellg();
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
    bool ret = compareVectors(h3hash, h3hashes.begin()->second);
    h0hashes.clear();
    h1hashes.clear();
    h2hashes.clear();
    h3hashes.clear();
    buffer.clear();
    overflowbuffer.clear();
    read = 0;
    block = 0;
    total_blocks = 0;
    blockCount = 0;
    return ret;
}