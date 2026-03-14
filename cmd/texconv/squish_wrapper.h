// squish_wrapper.h - C interface for libsquish
#ifndef SQUISH_WRAPPER_H
#define SQUISH_WRAPPER_H

#ifdef __cplusplus
extern "C" {
#endif

// Flags for compression
#define SQUISH_DXT1 (1 << 0)
#define SQUISH_DXT5 (1 << 2)
#define SQUISH_BC5  (1 << 4)
#define SQUISH_COLOUR_CLUSTER_FIT (1 << 5)

// Compress an RGBA image to BC format
void squish_compress_image(const unsigned char* rgba, int width, int height,
                           void* blocks, int flags);

// Calculate required storage size
int squish_get_storage_requirements(int width, int height, int flags);

// Decompress a BC image to RGBA
void squish_decompress_image(unsigned char* rgba, int width, int height,
                              const void* blocks, int flags);

#ifdef __cplusplus
}
#endif

#endif // SQUISH_WRAPPER_H
