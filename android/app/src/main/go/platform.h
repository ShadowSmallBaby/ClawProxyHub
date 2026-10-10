#pragma once
#include <stdint.h>
typedef struct { int requests; int callbacks; int64_t id; char *error; } CPHBinding;
CPHBinding CPHPlatformConnect(const char *name);
void CPHPlatformRelease(int64_t id);
char *CPHPlatformDiscover(void);

int CPHPlatformHasLua(void);
char *CPHPlatformComponents(void);
char *CPHPlatformExtensionTrust(void);
CPHBinding CPHPlatformConnectRuntime(const char *directory, const char *archive);
typedef struct { int socket; int64_t id; char *error; } CPHExtensionBinding;
CPHExtensionBinding CPHPlatformConnectExtension(const char *path, const char *digest, int min_sdk);
void CPHPlatformReleaseExtension(int64_t id);
