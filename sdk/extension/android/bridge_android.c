#include <jni.h>
#include "_cgo_export.h"

JNIEXPORT jlong JNICALL Java_github_shadowbaby_clawproxyhub_extension_ExtensionService_nativeOpen(JNIEnv *env, jclass type, jint socket) {
    return CPHExtensionOpen(socket);
}
JNIEXPORT void JNICALL Java_github_shadowbaby_clawproxyhub_extension_ExtensionService_nativeClose(JNIEnv *env, jclass type, jlong session) {
    CPHExtensionClose(session);
}
