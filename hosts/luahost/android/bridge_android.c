#include <jni.h>
#include "_cgo_export.h"
JNIEXPORT jlong JNICALL Java_github_shadowbaby_clawproxyhub_lua_LuaService_nativeOpen(JNIEnv *env,jclass type,jint requests,jint callbacks,jint bundle,jstring directory){const char *dir=(*env)->GetStringUTFChars(env,directory,NULL);if(!dir)return 0;jlong result=CPHLuaOpen(requests,callbacks,bundle,(char*)dir);(*env)->ReleaseStringUTFChars(env,directory,dir);return result;}
JNIEXPORT void JNICALL Java_github_shadowbaby_clawproxyhub_lua_LuaService_nativeClose(JNIEnv *env,jclass type,jlong id){CPHLuaClose(id);}
