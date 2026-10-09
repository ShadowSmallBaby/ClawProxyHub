#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include "platform.h"
#include "_cgo_export.h"
static JavaVM *vm;
static jclass platform;
JNIEXPORT jint JNICALL JNI_OnLoad(JavaVM *value,void *unused){vm=value;return JNI_VERSION_1_6;}
static JNIEnv *attach(int *attached){JNIEnv *env=NULL;*attached=0;if((*vm)->GetEnv(vm,(void**)&env,JNI_VERSION_1_6)!=JNI_OK){if((*vm)->AttachCurrentThread(vm,&env,NULL)!=JNI_OK)return NULL;*attached=1;}return env;}
static void detach(int attached){if(attached)(*vm)->DetachCurrentThread(vm);}
static jbyteArray result(JNIEnv *env,char *value){if(!value)return NULL;jsize len=strlen(value);jbyteArray out=(*env)->NewByteArray(env,len);if(out)(*env)->SetByteArrayRegion(env,out,0,len,(jbyte*)value);free(value);return out;}
// 将 Java 连接异常交回核心，避免把验签、身份和服务错误都隐藏为绑定失败。
static char *connectionError(JNIEnv *env){
 jthrowable error=(*env)->ExceptionOccurred(env);if(!error)return NULL;(*env)->ExceptionClear(env);
 char *out=NULL;jclass type=(*env)->GetObjectClass(env,error);
 jmethodID method=type?(*env)->GetMethodID(env,type,"toString","()Ljava/lang/String;"):NULL;
 jstring message=method?(jstring)(*env)->CallObjectMethod(env,error,method):NULL;
 if((*env)->ExceptionCheck(env)){(*env)->ExceptionClear(env);message=NULL;}
 if(message){const char *value=(*env)->GetStringUTFChars(env,message,NULL);if(value){out=strdup(value);(*env)->ReleaseStringUTFChars(env,message,value);}(*env)->DeleteLocalRef(env,message);}
 if((*env)->ExceptionCheck(env))(*env)->ExceptionClear(env);
 if(type)(*env)->DeleteLocalRef(env,type);(*env)->DeleteLocalRef(env,error);return out;
}
CPHBinding CPHPlatformConnect(const char *name){CPHBinding out={-1,-1,0,NULL};int attached;JNIEnv *env=attach(&attached);if(!env)return out;jmethodID method=(*env)->GetStaticMethodID(env,platform,"open","(Ljava/lang/String;)[J");jstring value=(*env)->NewStringUTF(env,name);jlongArray values=(jlongArray)(*env)->CallStaticObjectMethod(env,platform,method,value);if((*env)->ExceptionCheck(env)){out.error=connectionError(env);values=NULL;}if(values&&(*env)->GetArrayLength(env,values)==3){jlong data[3];(*env)->GetLongArrayRegion(env,values,0,3,data);out.requests=data[0];out.callbacks=data[1];out.id=data[2];}if(values)(*env)->DeleteLocalRef(env,values);(*env)->DeleteLocalRef(env,value);detach(attached);return out;}
void CPHPlatformRelease(int64_t id){int attached;JNIEnv *env=attach(&attached);if(!env)return;jmethodID method=(*env)->GetStaticMethodID(env,platform,"release","(J)V");(*env)->CallStaticVoidMethod(env,platform,method,(jlong)id);if((*env)->ExceptionCheck(env))(*env)->ExceptionClear(env);detach(attached);}
char *CPHPlatformDiscover(void){int attached;JNIEnv *env=attach(&attached);if(!env)return NULL;jmethodID method=(*env)->GetStaticMethodID(env,platform,"discover","()Ljava/lang/String;");jstring value=(jstring)(*env)->CallStaticObjectMethod(env,platform,method);if((*env)->ExceptionCheck(env)){(*env)->ExceptionClear(env);value=NULL;}char *out=NULL;if(value){const char *chars=(*env)->GetStringUTFChars(env,value,NULL);if(chars){out=strdup(chars);(*env)->ReleaseStringUTFChars(env,value,chars);}(*env)->DeleteLocalRef(env,value);}detach(attached);return out;}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_start(JNIEnv *env,jclass cls,jstring path){if(!platform)platform=(*env)->NewGlobalRef(env,cls);const char *directory=(*env)->GetStringUTFChars(env,path,NULL);if(!directory)return NULL;char *out=CPHCoreStart((char*)directory);(*env)->ReleaseStringUTFChars(env,path,directory);return result(env,out);}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_sync(JNIEnv *env,jclass cls){return result(env,CPHCoreSync());}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_stop(JNIEnv *env,jclass cls){return result(env,CPHCoreStop());}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_due(JNIEnv *env,jclass cls,jlong id){return result(env,CPHCoreDue(id));}
JNIEXPORT void JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_cancel(JNIEnv *env,jclass cls,jlong id){CPHCoreCancel(id);}

JNIEXPORT jlong JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_newJob(JNIEnv *env,jclass cls){return CPHCoreNewJob();}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_refresh(JNIEnv *env,jclass cls,jstring name){const char *value=(*env)->GetStringUTFChars(env,name,NULL);if(!value)return NULL;char *out=CPHCoreRefresh((char*)value);(*env)->ReleaseStringUTFChars(env,name,value);return result(env,out);}
JNIEXPORT jbyteArray JNICALL Java_github_shadowbaby_clawproxyhub_core_NativeCore_runtime(JNIEnv *env,jclass cls,jstring request){const char *value=(*env)->GetStringUTFChars(env,request,NULL);if(!value)return NULL;char *out=CPHCoreRuntime((char*)value);(*env)->ReleaseStringUTFChars(env,request,value);return result(env,out);}

int CPHPlatformHasLua(void){int attached;JNIEnv *env=attach(&attached);if(!env)return 0;jmethodID method=(*env)->GetStaticMethodID(env,platform,"hasLua","()Z");jboolean value=(*env)->CallStaticBooleanMethod(env,platform,method);if((*env)->ExceptionCheck(env)){(*env)->ExceptionClear(env);value=0;}detach(attached);return value;}

char *CPHPlatformComponents(void){int attached;JNIEnv *env=attach(&attached);if(!env)return NULL;jmethodID method=(*env)->GetStaticMethodID(env,platform,"systemComponents","()Ljava/lang/String;");jstring value=(jstring)(*env)->CallStaticObjectMethod(env,platform,method);if((*env)->ExceptionCheck(env)){(*env)->ExceptionClear(env);value=NULL;}char *out=NULL;if(value){const char *chars=(*env)->GetStringUTFChars(env,value,NULL);if(chars){out=strdup(chars);(*env)->ReleaseStringUTFChars(env,value,chars);}(*env)->DeleteLocalRef(env,value);}detach(attached);return out;}
char *CPHPlatformExtensionTrust(void){int attached;JNIEnv *env=attach(&attached);if(!env)return NULL;jmethodID method=(*env)->GetStaticMethodID(env,platform,"extensionTrust","()Ljava/lang/String;");jstring value=(jstring)(*env)->CallStaticObjectMethod(env,platform,method);if((*env)->ExceptionCheck(env)){(*env)->ExceptionClear(env);value=NULL;}char *out=NULL;if(value){const char *chars=(*env)->GetStringUTFChars(env,value,NULL);if(chars){out=strdup(chars);(*env)->ReleaseStringUTFChars(env,value,chars);}(*env)->DeleteLocalRef(env,value);}detach(attached);return out;}
CPHBinding CPHPlatformConnectRuntime(const char *directory,const char *archive){
 CPHBinding out={-1,-1,0,NULL};int attached;JNIEnv *env=attach(&attached);if(!env)return out;
 jmethodID method=(*env)->GetStaticMethodID(env,platform,"openRuntime","(Ljava/lang/String;Ljava/lang/String;)[J");
 jstring dir=(*env)->NewStringUTF(env,directory),path=(*env)->NewStringUTF(env,archive);
 jlongArray values=(jlongArray)(*env)->CallStaticObjectMethod(env,platform,method,dir,path);
 if((*env)->ExceptionCheck(env)){out.error=connectionError(env);values=NULL;}
 if(values&&(*env)->GetArrayLength(env,values)==3){jlong data[3];(*env)->GetLongArrayRegion(env,values,0,3,data);out.requests=data[0];out.callbacks=data[1];out.id=data[2];}
 if(values)(*env)->DeleteLocalRef(env,values);(*env)->DeleteLocalRef(env,dir);(*env)->DeleteLocalRef(env,path);detach(attached);return out;
}

CPHExtensionBinding CPHPlatformConnectExtension(const char *path,const char *digest,int min_sdk){
 CPHExtensionBinding out={-1,0,NULL};int attached;JNIEnv *env=attach(&attached);if(!env)return out;
 jmethodID method=(*env)->GetStaticMethodID(env,platform,"openExtension","(Ljava/lang/String;Ljava/lang/String;I)[J");
 jstring library=(*env)->NewStringUTF(env,path),hash=(*env)->NewStringUTF(env,digest);
 jlongArray values=NULL;
 if(method&&library&&hash)values=(jlongArray)(*env)->CallStaticObjectMethod(env,platform,method,library,hash,(jint)min_sdk);
 if((*env)->ExceptionCheck(env)){out.error=connectionError(env);values=NULL;}
 if(values&&(*env)->GetArrayLength(env,values)==2){jlong data[2];(*env)->GetLongArrayRegion(env,values,0,2,data);out.socket=data[0];out.id=data[1];}
 if(values)(*env)->DeleteLocalRef(env,values);
 if(library)(*env)->DeleteLocalRef(env,library);
 if(hash)(*env)->DeleteLocalRef(env,hash);
 detach(attached);return out;
}
void CPHPlatformReleaseExtension(int64_t id){
 int attached;JNIEnv *env=attach(&attached);if(!env)return;
 jmethodID method=(*env)->GetStaticMethodID(env,platform,"releaseExtension","(J)V");
 if(method)(*env)->CallStaticVoidMethod(env,platform,method,(jlong)id);
 if((*env)->ExceptionCheck(env))(*env)->ExceptionClear(env);
 detach(attached);
}
