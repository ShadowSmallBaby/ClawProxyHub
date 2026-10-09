package github.shadowbaby.clawproxyhub.core;
import android.app.*;import android.os.*;import android.content.*;import android.util.Log;import org.json.*;import java.net.*;import java.nio.charset.StandardCharsets;
// 独立测试 APK 经同签名宿主运行，报告不包含会话 Token。
public final class CoreInstrumentation extends Instrumentation {
 private boolean verifyProgressUI;
 private boolean verifyHostPackage;
 static void verifyCapabilities(JSONObject capabilities) throws Exception {
  if (!"full".equals(capabilities.getString("profile"))) throw new AssertionError("wrong core profile");
  JSONArray modules = capabilities.getJSONArray("modules");
  for (int i = 0; i < modules.length(); i++) if ("web".equals(modules.getJSONObject(i).getString("id"))) throw new AssertionError("Android core must not serve embedded Web");
 }
 private boolean verifyExtension,verifyMarket,verifyOnboarding,verifyLua,verifyInstall,verifyApplication,verifyFrontend,verifyWindow,verifyWorkspace;private String backgroundPhase,pluginName,nativePhase;
 public void onCreate(Bundle arguments){super.onCreate(arguments);verifyProgressUI="true".equals(arguments.getString("progress_ui"));verifyHostPackage="true".equals(arguments.getString("host_package"));verifyExtension="true".equals(arguments.getString("extension"));verifyMarket="true".equals(arguments.getString("market"));verifyOnboarding="true".equals(arguments.getString("onboarding"));verifyWorkspace="true".equals(arguments.getString("workspace"));verifyLua="true".equals(arguments.getString("lua"));verifyInstall="true".equals(arguments.getString("install"));verifyApplication="true".equals(arguments.getString("application"));verifyFrontend="true".equals(arguments.getString("frontend"));verifyWindow="true".equals(arguments.getString("window"));backgroundPhase=arguments.getString("background");pluginName=arguments.getString("plugin");nativePhase=arguments.getString("native");start();}
 public void onStart(){Bundle result=new Bundle();try{
  if(verifyProgressUI){result.putString("report",InstallProgressChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyHostPackage){result.putString("report",HostPackageChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyExtension){result.putString("report",ExtensionChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyMarket){result.putString("report",PluginMarketChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyOnboarding){result.putString("report",OnboardingChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyWorkspace){result.putString("report",WorkspaceChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(nativePhase!=null){result.putString("report",NativePluginChecks.run(this,nativePhase,pluginName).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyWindow){result.putString("report",WindowChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(verifyFrontend){FrontendChecks.run(this);result.putString("report","signed frontend, tamper rejection, crash fallback and builtin reset passed");finish(Activity.RESULT_OK,result);return;}
  if(verifyApplication){result.putString("report",ApplicationChecks.run(this).toString());finish(Activity.RESULT_OK,result);return;}
  if(pluginName!=null){result.putString("report",ApplicationChecks.plugin(this,pluginName).toString());finish(Activity.RESULT_OK,result);return;}
  if(backgroundPhase!=null){result.putString("report",BackgroundChecks.run(this,backgroundPhase).toString());finish(Activity.RESULT_OK,result);return;}
  NativePluginChecks.run(this,"fixture","androidtest");
  Context context=getTargetContext();NativeCore.initialize(context);JSONObject session=NativeCore.session();String base=session.getString("address"),token=session.getString("token");
  verifyCapabilities(request(base+"/admin/capabilities",token));
  NativeCore.decode(NativeCore.sync());
  TaskScheduler.schedule(context);if(context.getSystemService(android.app.job.JobScheduler.class).getPendingJob(1001)==null)throw new AssertionError("system scheduler not registered");
  if(verifyLua)verifyLua(base,token);if(verifyInstall)verifyInstallation(base,token);verifyBackup(base,token);
  NativeCore.decode(NativeCore.due(NativeCore.newJob()));NativeCore.decode(NativeCore.stop());NativeCore.session();
  result.putString("lua",verifyLua?"passed":"not-requested");result.putString("report","{\"ok\":true,\"checks\":[\"SQLite/core-start\",\"authenticated-loopback\",\"full-core-without-web\",\"signed-plugin-Service\",\"system-scheduler\",\"core-restart\"]}");finish(Activity.RESULT_OK,result);
 }catch(Throwable error){Log.e("CPH-instrumentation","core verification failed",error);result.putString("error",error.toString());finish(Activity.RESULT_CANCELED,result);}}
 void verifyLua(String base,String token)throws Exception{
  if(!NativeCore.hasLua())throw new AssertionError("Bundled Lua service missing");String name="lua-check-"+System.nanoTime();
  String source="local PLUGIN_NAME = \""+name+"\"\nlocal M={}\nfunction M.task(req) cph.log.info('Android Lua callback'); return {summary='lua-service-ok'} end\nreturn M";
  request(base+"/admin/actions/core.workspace.create",token,"POST",new JSONObject().put("name",name).put("content",source));
  request(base+"/admin/actions/core.workspace.reload",token,"POST",new JSONObject().put("name",name));
  JSONArray plugins=request(base+"/admin/plugins",token).getJSONArray("plugins");long pluginId=0;for(int i=0;i<plugins.length();i++){JSONObject plugin=plugins.getJSONObject(i);if(name.equals(plugin.getString("name"))&&plugin.getBoolean("running"))pluginId=plugin.getLong("id");}if(pluginId==0)throw new AssertionError("Lua plugin did not start");
  long rule=request(base+"/admin/task-rules",token,"POST",new JSONObject().put("plugin_id",pluginId).put("capability_id","echo").put("trigger_type","interval").put("trigger_value","1h").put("target_scope","global")).getLong("id");
  long run=request(base+"/admin/task-rules/"+rule+"/run",token,"POST",new JSONObject()).getLong("run_id");boolean completed=false;
  for(int retry=0;retry<100&&!completed;retry++){JSONArray runs=request(base+"/admin/task-runs",token).getJSONArray("runs");for(int i=0;i<runs.length();i++){JSONObject item=runs.getJSONObject(i);if(item.getLong("id")==run){if(item.getString("status").equals("failed"))throw new AssertionError(item.toString());completed="lua-service-ok".equals(item.optString("summary"));}}if(!completed)Thread.sleep(50);}
  if(!completed)throw new AssertionError("Lua task did not complete");
  String slow="local PLUGIN_NAME = \""+name+"\"\nlocal M={}\nfunction M.task(req) cph.time.sleep(10000); return {summary='unexpected-success'} end\nreturn M";
  request(base+"/admin/actions/core.workspace.save",token,"POST",new JSONObject().put("name",name).put("file","main.lua").put("content",slow));request(base+"/admin/actions/core.workspace.reload",token,"POST",new JSONObject().put("name",name));
  long cancelled=request(base+"/admin/actions/core.tasks.run",token,"POST",new JSONObject().put("id",rule)).getLong("run_id");Thread.sleep(300);request(base+"/admin/actions/core.tasks.cancel",token,"POST",new JSONObject().put("id",rule));boolean stopped=false;
  for(int retry=0;retry<100&&!stopped;retry++){JSONArray runs=request(base+"/admin/task-runs",token).getJSONArray("runs");for(int i=0;i<runs.length();i++){JSONObject item=runs.getJSONObject(i);if(item.getLong("id")==cancelled&&"failed".equals(item.getString("status"))){if(!item.optString("error_message").toLowerCase().contains("cancel"))throw new AssertionError("unexpected task failure: "+item);stopped=true;}}if(!stopped)Thread.sleep(50);}if(!stopped)throw new AssertionError("task cancellation did not settle");
  request(base+"/admin/task-rules/"+rule,token,"DELETE",null);request(base+"/admin/plugins/"+name,token,"DELETE",null);
  Log.i("CPH-instrumentation","Lua task cancellation passed");Log.i("CPH-instrumentation","Lua Service task and host callback passed");
 }
 private void verifyInstallation(String base,String token)throws Exception{
  java.io.File source=new java.io.File(getTargetContext().getCacheDir(),"plugin.cphplugin");
  String name=new NativePluginStore(getTargetContext()).install(android.net.Uri.fromFile(source));
  NativeCore.decode(NativeCore.refresh(name));
  ApplicationChecks.plugin(this,name);
 }
 private void verifyBackup(String base,String token)throws Exception{
  String original=request(base+"/admin/settings",token).getJSONObject("settings").getString("site_name");
  request(base+"/admin/settings",token,"PUT",new JSONObject().put("site_name","android-backup-check"));
  HttpURLConnection connection=(HttpURLConnection)new URL(base+"/admin/system/backup").openConnection();connection.setRequestProperty("Authorization","Bearer "+token);byte[] backup;try(java.io.InputStream input=connection.getInputStream()){backup=NativeCore.readBounded(input,16*1024*1024);}finally{connection.disconnect();}
  request(base+"/admin/settings",token,"PUT",new JSONObject().put("site_name","changed-after-backup"));multipart(base+"/admin/system/restore",token,"file",backup);
  NativeCore.decode(NativeCore.stop());JSONObject session=NativeCore.session();base=session.getString("address");token=session.getString("token");
  if(!"android-backup-check".equals(request(base+"/admin/settings",token).getJSONObject("settings").getString("site_name")))throw new AssertionError("backup restore did not persist settings");
  request(base+"/admin/settings",token,"PUT",new JSONObject().put("site_name",original));Log.i("CPH-instrumentation","backup export, staged restore and restarted SQLite passed");
 }
 JSONObject multipart(String address,String token,String field,byte[] bytes)throws Exception{
  return multipart(address,token,field,bytes,null);
 }
 JSONObject multipart(String address,String token,String field,byte[] bytes,JSONArray grants)throws Exception{
  String boundary="cph-test-boundary";HttpURLConnection connection=(HttpURLConnection)new URL(address).openConnection();connection.setRequestMethod("POST");connection.setConnectTimeout(5000);connection.setReadTimeout(15000);connection.setDoOutput(true);connection.setRequestProperty("Authorization","Bearer "+token);connection.setRequestProperty("Content-Type","multipart/form-data; boundary="+boundary);
  try(java.io.OutputStream output=connection.getOutputStream()){output.write(("--"+boundary+"\r\nContent-Disposition: form-data; name=\""+field+"\"; filename=\"fixture.zip\"\r\nContent-Type: application/zip\r\n\r\n").getBytes(StandardCharsets.UTF_8));output.write(bytes);if(grants!=null)output.write(("\r\n--"+boundary+"\r\nContent-Disposition: form-data; name=\"grants\"\r\n\r\n"+grants).getBytes(StandardCharsets.UTF_8));output.write(("\r\n--"+boundary+"--\r\n").getBytes(StandardCharsets.UTF_8));}
  try{int code=connection.getResponseCode();byte[] raw=NativeCore.readBounded(code>=400?connection.getErrorStream():connection.getInputStream(),2*1024*1024);if(code>=400)throw new IllegalStateException(new String(raw,StandardCharsets.UTF_8));return new JSONObject(new String(raw,StandardCharsets.UTF_8));}finally{connection.disconnect();}
 }
 JSONObject request(String address,String token)throws Exception{return request(address,token,"GET",null);}
 JSONObject request(String address,String token,String method,JSONObject data)throws Exception{HttpURLConnection c=(HttpURLConnection)new URL(address).openConnection();c.setRequestMethod(method);c.setRequestProperty("Authorization","Bearer "+token);c.setConnectTimeout(5000);c.setReadTimeout(15000);if(data!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(java.io.OutputStream output=c.getOutputStream()){output.write(data.toString().getBytes(StandardCharsets.UTF_8));}}try{int status=c.getResponseCode();if(status>=400)throw new IllegalStateException(new String(NativeCore.readBounded(c.getErrorStream(),2*1024*1024),StandardCharsets.UTF_8));return new JSONObject(new String(NativeCore.readBounded(c.getInputStream(),2*1024*1024),StandardCharsets.UTF_8));}finally{c.disconnect();}}
}
