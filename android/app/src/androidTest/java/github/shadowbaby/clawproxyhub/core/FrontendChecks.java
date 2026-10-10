package github.shadowbaby.clawproxyhub.core;
import android.net.Uri;
import java.io.*;

// 测试签名更新、崩溃回退及用户显式恢复内置资源，统一由主应用 runner 分派。
final class FrontendChecks {
 static void run(CoreInstrumentation test)throws Exception{
  FrontendUpdates updates=new FrontendUpdates(test.getTargetContext());
  updates.install(copy(test,"good"));if(!updates.pending()||updates.open("web/index.html")==null)throw new AssertionError("signed frontend did not load");updates.confirm();
  boolean rejected=false;try{updates.install(copy(test,"tampered"));}catch(SecurityException expected){rejected=true;}if(!rejected)throw new AssertionError("tampered frontend accepted");
  updates.install(copy(test,"pending"));if(!updates.pending())throw new AssertionError("activation not pending");FrontendUpdates restarted=new FrontendUpdates(test.getTargetContext());
  String restored=new String(read(restarted.open("web/index.html").getData()),java.nio.charset.StandardCharsets.UTF_8);if(!restored.contains("good")||restarted.pending())throw new AssertionError("failed frontend did not roll back");
  restarted.install(copy(test,"pending"));restarted.confirm();restarted.reset();if(restarted.open("web/index.html")!=null)throw new AssertionError("builtin fallback unavailable");
 }
 private static Uri copy(CoreInstrumentation test,String name)throws Exception{File out=new File(test.getTargetContext().getCacheDir(),name+".cphui");try(InputStream input=test.getContext().getAssets().open(name+".cphui");OutputStream output=new FileOutputStream(out)){output.write(read(input));}return Uri.fromFile(out);}
 private static byte[] read(InputStream input)throws Exception{try(InputStream in=input){ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] buffer=new byte[16384];int n;while((n=in.read(buffer))!=-1)out.write(buffer,0,n);return out.toByteArray();}}
}
