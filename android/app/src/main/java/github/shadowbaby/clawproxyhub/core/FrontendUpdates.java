package github.shadowbaby.clawproxyhub.core;
import android.content.*;import android.net.Uri;import android.webkit.*;import org.json.*;import java.io.*;import java.util.*;import java.util.zip.*;

import static github.shadowbaby.clawproxyhub.core.FrontendPackage.*;

// 完整界面只接受 APP 发行证书；更新失败回退内置资源。
final class FrontendUpdates {
 private final Context context;private final android.content.SharedPreferences state;private final File root;private JSONObject manifest;private File selected;
 FrontendUpdates(Context context){this.context=context;state=context.getSharedPreferences("frontend-updates",0);root=new File(context.getFilesDir(),"frontends");root.mkdirs();if(state.getBoolean("pending",false))rollback();load();}
 private static byte[] read(InputStream in,int max)throws IOException{ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[16384];int n;while((n=in.read(buffer))!=-1){if(out.size()+n>max)throw new IOException("frontend package exceeds limit");out.write(buffer,0,n);}return out.toByteArray();}
 private Map<String,byte[]> verify(File file)throws Exception{
  Map<String,byte[]> files=new HashMap<>();Set<String> seen=new HashSet<>();int total=0;
  try(ZipFile zip=new ZipFile(file)){
   Enumeration<? extends ZipEntry> entries=zip.entries();
   while(entries.hasMoreElements()){
    ZipEntry entry=entries.nextElement();String name=entry.getName();if(entry.isDirectory())continue;
    if(!safe(name)||name.equalsIgnoreCase("package.cphui")||!seen.add(name.toLowerCase(Locale.ROOT))||files.size()>=4096)throw new SecurityException("Unsafe frontend package path");
    byte[] content;try(InputStream input=zip.getInputStream(entry)){content=read(input,32*1024*1024-total);}total+=content.length;files.put(name,content);
   }
  }
  byte[] signed=files.get("signature.json");if(signed==null||signed.length>16384)throw new SecurityException("Missing frontend signature");
  JSONObject signature=new JSONObject(new String(signed,java.nio.charset.StandardCharsets.UTF_8));
  String abi=android.os.Build.SUPPORTED_ABIS[0],platform="android/"+(abi.equals("arm64-v8a")?"arm64":abi.equals("x86_64")?"amd64":"unsupported");
  FrontendPackage.verify(files,AppSignature.trustedKey(context,signature),android.util.Base64.decode(signature.getString("value"),0),BuildConfig.CORE_VERSION,platform,BuildConfig.FRONTEND_VERSION);
  return files;
 }
 synchronized void install(Uri uri)throws Exception{
  File temporary=File.createTempFile("frontend-",".cphui",root);try{byte[] archive;try(InputStream input=context.getContentResolver().openInputStream(uri)){archive=read(input,64*1024*1024);}try(OutputStream out=new FileOutputStream(temporary)){out.write(archive);}Map<String,byte[]> files=verify(temporary);JSONObject next=new JSONObject(new String(files.get("manifest.json"),java.nio.charset.StandardCharsets.UTF_8));if(manifest!=null&&compare(next.getString("version"),manifest.getString("version"))<0)throw new SecurityException("frontend downgrade refused");
   String id=hash(archive);if(manifest!=null&&compare(next.getString("version"),manifest.getString("version"))==0&&!id.equals(state.getString("current","")))throw new SecurityException("Published frontend versions are immutable");File dir=new File(root,id);if(!dir.exists()){File stage=new File(root,"stage-"+UUID.randomUUID());stage.mkdir();try{for(Map.Entry<String,byte[]> entry:files.entrySet()){File target=new File(stage,entry.getKey());target.getParentFile().mkdirs();try(OutputStream out=new FileOutputStream(target)){out.write(entry.getValue());}}try(OutputStream out=new FileOutputStream(new File(stage,"package.cphui"))){out.write(archive);}if(!stage.renameTo(dir))throw new IOException("cannot activate frontend directory");}finally{clean(stage);}}
   if(!state.edit().putString("previous",state.getString("current","")).putString("current",id).putBoolean("pending",true).commit())throw new IOException("cannot persist frontend activation");load();
  }finally{temporary.delete();}
 }
 private synchronized void load(){String id=state.getString("current","");selected=null;manifest=null;if(id.isEmpty())return;if(!id.matches("[0-9a-f]{64}")){rollback();return;}try{File dir=new File(root,id);File archive=new File(dir,"package.cphui");try(InputStream input=new FileInputStream(archive)){if(!id.equals(hash(read(input,64*1024*1024))))throw new SecurityException("frontend archive identity mismatch");}Map<String,byte[]> files=verify(archive);for(Map.Entry<String,byte[]> entry:files.entrySet()){try(InputStream input=new FileInputStream(new File(dir,entry.getKey()))){if(!java.util.Arrays.equals(entry.getValue(),read(input,32*1024*1024)))throw new SecurityException("stored frontend resource modified");}}manifest=new JSONObject(new String(files.get("manifest.json"),java.nio.charset.StandardCharsets.UTF_8));selected=dir;}catch(Exception error){state.edit().remove("current").remove("previous").putBoolean("pending",false).commit();}}
 synchronized void confirm(){if(!state.edit().putBoolean("pending",false).commit())throw new IllegalStateException("cannot confirm frontend activation");}
 synchronized boolean pending(){return state.getBoolean("pending",false);}
 synchronized void reset(){if(!state.edit().remove("current").remove("previous").putBoolean("pending",false).commit())throw new IllegalStateException("cannot restore builtin frontend");load();}
 synchronized void rollback(){String previous=state.getString("previous","");if(!state.edit().putString("current",previous).remove("previous").putBoolean("pending",false).commit())throw new IllegalStateException("cannot persist frontend rollback");load();}
 synchronized WebResourceResponse open(String path){if(selected==null||!path.startsWith("web/")||!safe(path))return null;String name="frontend/"+path.substring(4);try{String expected=manifest.getJSONObject("files").getString(name);byte[] data;try(InputStream input=new FileInputStream(new File(selected,name))){data=read(input,32*1024*1024);}if(!expected.equals(hash(data)))throw new SecurityException("installed frontend modified");String type=name.endsWith(".js")?"application/javascript":name.endsWith(".css")?"text/css":name.endsWith(".html")?"text/html":java.net.URLConnection.guessContentTypeFromName(name);return new WebResourceResponse(type==null?"application/octet-stream":type,"UTF-8",new ByteArrayInputStream(data));}catch(Exception error){return new WebResourceResponse("text/plain","UTF-8",404,"Missing signed resource",Collections.emptyMap(),new ByteArrayInputStream(new byte[0]));}}
 private static void clean(File dir){File[] entries=dir.listFiles();if(entries!=null)for(File child:entries){if(child.isDirectory())clean(child);else child.delete();}dir.delete();}
}
