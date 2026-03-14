// squish_wrapper.cpp - C wrapper for libsquish C++ API
#include <squish.h>
#include "squish_wrapper.h"

extern "C" {

void squish_compress_image(const unsigned char* rgba, int width, int height, 
                           void* blocks, int flags) {
    squish::CompressImage(rgba, width, height, blocks, flags, nullptr);
}

int squish_get_storage_requirements(int width, int height, int flags) {
    return squish::GetStorageRequirements(width, height, flags);
}

void squish_decompress_image(unsigned char* rgba, int width, int height,
                              const void* blocks, int flags) {
    squish::DecompressImage(rgba, width, height, blocks, flags);
}

}  // extern "C"
