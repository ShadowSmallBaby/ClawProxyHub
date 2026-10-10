package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Build;
import android.util.Base64;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.*;
import java.util.zip.*;
import org.json.*;

// 宿主管理签名原生包；只有与宿主发行证书一致的发布者可提供同 UID 原生代码。
public final class NativePluginStore {
  private static final Object LOCK = new Object();
  private final Context context;
  private final File root;
  private final SharedPreferences preferences;

  public NativePluginStore(Context context) {
    this.context = context.getApplicationContext();
    root = new File(context.getFilesDir(), "native-plugins");
    preferences = context.getSharedPreferences("native-plugins", 0);
  }

  public static byte[] read(InputStream input, int max) throws IOException {
    if (input == null) throw new IOException("Missing file");
    ByteArrayOutputStream result = new ByteArrayOutputStream();
    byte[] buffer = new byte[32768];
    int count;
    while ((count = input.read(buffer)) != -1) {
      if (result.size() + count > max) throw new IOException("Plugin package exceeds size limit");
      result.write(buffer, 0, count);
    }
    return result.toByteArray();
  }

  static String hash(byte[] bytes) throws Exception {
    StringBuilder result = new StringBuilder();
    for (byte b : MessageDigest.getInstance("SHA-256").digest(bytes))
      result.append(String.format(Locale.ROOT, "%02x", b & 255));
    return result.toString();
  }

  private static boolean safe(String path) {
    if (path.isEmpty() || path.startsWith("/") || path.contains("\\") || path.contains(":"))
      return false;
    for (String part : path.split("/", -1))
      if (part.isEmpty()
          || part.equals(".")
          || part.equals("..")
          || part.endsWith(".")
          || part.endsWith(" ")) return false;
    return true;
  }

  private static String name(String value) {
    if (!value.matches("[a-z][a-z0-9_]{0,63}"))
      throw new SecurityException("Invalid plugin identity");
    return value;
  }

  private static long[] version(String value) {
    if (!value.matches("(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)"))
      throw new SecurityException("Invalid plugin version");
    String[] parts = value.split("\\.");
    return new long[] {
      Long.parseLong(parts[0]), Long.parseLong(parts[1]), Long.parseLong(parts[2])
    };
  }

  private static int compare(String a, String b) {
    long[] x = version(a), y = version(b);
    for (int i = 0; i < 3; i++) {
      int result = Long.compare(x[i], y[i]);
      if (result != 0) return result;
    }
    return 0;
  }

  // 运行时身份取自包 ID；name 是可本地化的展示名称。
  static void validateIdentity(JSONObject manifest, boolean hostPackage) throws Exception {
    version(manifest.getString("version"));
    if (!hostPackage) {
      name(manifest.getString("name"));
      if (manifest.optInt("protocol_version") != 2
          || !"go".equals(manifest.optString("runtime", "go")))
        throw new SecurityException("Incompatible business plugin protocol");
      return;
    }
    if (!"lua-runtime".equals(manifest.optString("id"))
        || !"runtime".equals(manifest.optString("kind"))
        || manifest.optInt("api") != 1
        || !"backend".equals(manifest.optString("target"))
        || !"android-service".equals(manifest.optString("execution"))
        || manifest.optString("name").trim().isEmpty()
        || manifest.has("runtime")
        || manifest.has("protocol_version"))
      throw new SecurityException("Invalid Lua Host package identity");
  }

  private Map<String, byte[]> verify(File archive) throws Exception {
    return verify(archive, false);
  }

  Map<String, byte[]> verify(File archive, boolean hostPackage) throws Exception {
    if (archive.length() > 128 * 1024 * 1024)
      throw new SecurityException("Plugin package too large");
    Map<String, byte[]> files = new LinkedHashMap<>();
    Set<String> seen = new HashSet<>();
    int total = 0;
    try (ZipFile zip = new ZipFile(archive)) {
      Enumeration<? extends ZipEntry> entries = zip.entries();
      while (entries.hasMoreElements()) {
        ZipEntry entry = entries.nextElement();
        String path = entry.getName();
        if (entry.isDirectory()) continue;
        if (!safe(path) || !seen.add(path.toLowerCase(Locale.ROOT)) || files.size() >= 256)
          throw new SecurityException("Unsafe or duplicate plugin path");
        byte[] bytes;
        try (InputStream input = zip.getInputStream(entry)) {
          bytes = read(input, 128 * 1024 * 1024 - total);
        }
        total += bytes.length;
        files.put(path, bytes);
      }
    }
    byte[] raw = files.get("manifest.json"), signature = files.get("signature.json");
    if (raw == null || raw.length > 1024 * 1024 || signature == null || signature.length > 16384)
      throw new SecurityException("Missing signed manifest");
    JSONObject manifest = new JSONObject(new String(raw, StandardCharsets.UTF_8));
    JSONObject signed = new JSONObject(new String(signature, StandardCharsets.UTF_8));
    PackageSignature.verify(
        raw,
        AppSignature.trustedKey(context, signed),
        Base64.decode(signed.getString("value"), Base64.DEFAULT));
    validateIdentity(manifest, hostPackage);
    JSONObject android = manifest.getJSONObject("android"), hashes = android.getJSONObject("files");
    String abi = android.getString("abi"), library = android.getString("library");
    if (!(hostPackage ? "cph-host-v1" : "cph-native-v1").equals(android.getString("format"))
        || android.getInt("min_sdk") > Build.VERSION.SDK_INT
        || !Arrays.asList(Build.SUPPORTED_64_BIT_ABIS).contains(abi)
        || !library.equals("lib/" + abi + (hostPackage ? "/libcphlua.so" : "/libcphplugin.so"))
        || !hashes.has(library)
        || files.size() != hashes.length() + 2)
      throw new SecurityException("Incompatible plugin platform or contract");
    if (hostPackage) {
      JSONObject declared = manifest.getJSONObject("files");
      if (!library.equals(manifest.getString("entry")) || declared.length() != hashes.length())
        throw new SecurityException("Lua Host payload declarations differ");
      for (Iterator<String> paths = hashes.keys(); paths.hasNext(); ) {
        String path = paths.next();
        if (!hashes.getString(path).equals(declared.optString(path)))
          throw new SecurityException("Lua Host payload declarations differ");
      }
    }
    for (Iterator<String> paths = hashes.keys(); paths.hasNext(); ) {
      String path = paths.next();
      if (!safe(path)
          || !files.containsKey(path)
          || !hash(files.get(path)).equals(hashes.getString(path)))
        throw new SecurityException("Plugin content was modified");
    }
    byte[] elf = files.get(library);
    int machine = "arm64-v8a".equals(abi) ? 183 : 62;
    if (elf.length < 64
        || elf[0] != 127
        || elf[1] != 'E'
        || elf[2] != 'L'
        || elf[3] != 'F'
        || elf[4] != 2
        || elf[5] != 1
        || (elf[18] & 255) != machine
        || elf[19] != 0) throw new SecurityException("Native library ABI mismatch");
    return files;
  }

  private File directory(String id) {
    if (!id.matches("[0-9a-f]{64}"))
      throw new SecurityException("Invalid installed package identity");
    return new File(root, id);
  }

  private JSONObject installed(String plugin, boolean checkFiles) throws Exception {
    String id = preferences.getString("current:" + name(plugin), "");
    if (id.isEmpty()) throw new FileNotFoundException("Plugin not installed: " + plugin);
    return verifiedInstallation(plugin, id, checkFiles);
  }

  private JSONObject verifiedInstallation(String plugin, String id, boolean checkFiles)
      throws Exception {
    File dir = directory(id), archive = new File(dir, "package.cphplugin");
    byte[] archiveBytes;
    try (InputStream input = new FileInputStream(archive)) {
      archiveBytes = read(input, 128 * 1024 * 1024);
    }
    if (!id.equals(hash(archiveBytes)))
      throw new SecurityException("Stored package identity changed");
    Map<String, byte[]> files = verify(archive);
    JSONObject manifest =
        new JSONObject(new String(files.get("manifest.json"), StandardCharsets.UTF_8));
    if (!plugin.equals(manifest.getString("name")))
      throw new SecurityException("Installed plugin identity mismatch");
    if (checkFiles)
      for (Map.Entry<String, byte[]> entry : files.entrySet()) {
        try (InputStream input = new FileInputStream(new File(dir, entry.getKey()))) {
          if (!MessageDigest.isEqual(entry.getValue(), read(input, 128 * 1024 * 1024)))
            throw new SecurityException("Installed plugin content changed");
        }
      }
    return manifest;
  }

  public String library(String plugin) throws Exception {
    synchronized (LOCK) {
      JSONObject manifest = installed(plugin, true);
      if (!preferences.getBoolean("enabled:" + plugin, true))
        throw new IllegalStateException("Plugin disabled");
      return new File(
              directory(preferences.getString("current:" + plugin, "")),
              manifest.getJSONObject("android").getString("library"))
          .getAbsolutePath();
    }
  }

  // 管理列表只读元数据，不抢占安装/装载锁；执行与回滚仍完整校验签名及文件。
  public JSONArray list() throws Exception {
    JSONArray output = new JSONArray();
    Map<String, ?> snapshot = preferences.getAll();
    for (String key : new TreeSet<>(snapshot.keySet()))
      if (key.startsWith("current:")) {
        String plugin = key.substring(8);
        JSONObject value;
        try (InputStream input =
            new FileInputStream(new File(directory((String) snapshot.get(key)), "manifest.json"))) {
          value = new JSONObject(new String(read(input, 1024 * 1024), StandardCharsets.UTF_8));
          if (!name(plugin).equals(value.getString("name")))
            throw new SecurityException("Installed plugin identity mismatch");
          value.put("available", true);
          InstalledPlugins.attachIcon(value, directory((String) snapshot.get(key)));
        } catch (Exception error) {
          value =
              new JSONObject()
                  .put("name", plugin)
                  .put("version", "")
                  .put("available", false)
                  .put("error", error.getMessage());
        }
        value.put("enabled", !Boolean.FALSE.equals(snapshot.get("enabled:" + plugin)));
        value.put("rollback", snapshot.containsKey("previous:" + plugin));
        output.put(value);
      }
    return output;
  }

  public String discover() throws Exception {
    synchronized (LOCK) {
      JSONArray output = new JSONArray();
      for (String key : new TreeSet<>(preferences.getAll().keySet()))
        if (key.startsWith("current:")) {
          String plugin = key.substring(8);
          if (!preferences.getBoolean("enabled:" + plugin, true)) continue;
          try {
            JSONObject manifest = installed(plugin, false);
            String icon = manifest.optString("icon");
            if (!icon.isEmpty()) {
              File versionDir = directory(preferences.getString("current:" + plugin, ""));
              byte[] bytes;
              try (InputStream input = new FileInputStream(new File(versionDir, icon))) {
                bytes = read(input, 2 * 1024 * 1024);
              }
              if (!hash(bytes)
                  .equals(manifest.getJSONObject("android").getJSONObject("files").getString(icon)))
                throw new SecurityException("Installed icon changed");
              File mirrored =
                  new File(
                      context.getFilesDir(),
                      "plugins/system/" + plugin + "/" + new File(icon).getName());
              mirrored.getParentFile().mkdirs();
              android.util.AtomicFile file = new android.util.AtomicFile(mirrored);
              FileOutputStream stream = null;
              try {
                stream = file.startWrite();
                stream.write(bytes);
                file.finishWrite(stream);
              } catch (Exception error) {
                file.failWrite(stream);
                throw error;
              }
            }
            output.put(manifest);
          } catch (Exception ignored) {
            /* 损坏的安装仍显示在管理页，但不供核心加载。 */
          }
        }
      return output.toString();
    }
  }

  String install(Uri uri) throws Exception {
    synchronized (LOCK) {
      if (!root.isDirectory() && !root.mkdirs())
        throw new IOException("Cannot create plugin storage");
      File temporary = File.createTempFile("import-", ".cphplugin", root);
      try {
        byte[] archive;
        try (InputStream input = context.getContentResolver().openInputStream(uri)) {
          archive = read(input, 128 * 1024 * 1024);
        }
        try (FileOutputStream output = new FileOutputStream(temporary)) {
          output.write(archive);
          output.getFD().sync();
        }
        Map<String, byte[]> files = verify(temporary);
        JSONObject manifest =
            new JSONObject(new String(files.get("manifest.json"), StandardCharsets.UTF_8));
        String plugin = name(manifest.getString("name"));
        String previous = preferences.getString("current:" + plugin, ""), id = hash(archive);
        if (!previous.isEmpty()
            && compare(manifest.getString("version"), installed(plugin, false).getString("version"))
                < 0) throw new SecurityException("Plugin downgrade refused; use rollback instead");
        File destination = directory(id);
        if (!destination.isDirectory()) {
          File stage = new File(root, "stage-" + UUID.randomUUID());
          if (!stage.mkdir()) throw new IOException("Cannot stage plugin");
          try {
            for (Map.Entry<String, byte[]> entry : files.entrySet()) {
              File target = new File(stage, entry.getKey());
              target.getParentFile().mkdirs();
              try (FileOutputStream output = new FileOutputStream(target)) {
                output.write(entry.getValue());
                output.getFD().sync();
              }
              if (!target.setReadOnly()) throw new IOException("Cannot protect plugin content");
            }
            try (FileOutputStream output =
                new FileOutputStream(new File(stage, "package.cphplugin"))) {
              output.write(archive);
              output.getFD().sync();
            }
            if (!stage.renameTo(destination)) throw new IOException("Cannot activate plugin files");
          } finally {
            clean(stage);
          }
        }
        verifiedInstallation(plugin, id, true);
        SharedPreferences.Editor edit =
            preferences
                .edit()
                .putString("current:" + plugin, id)
                .putBoolean("enabled:" + plugin, true);
        if (!previous.isEmpty() && !previous.equals(id))
          edit.putString("previous:" + plugin, previous);
        commit(edit);
        return plugin;
      } finally {
        temporary.delete();
      }
    }
  }

  void enabled(String plugin, boolean enabled) throws Exception {
    synchronized (LOCK) {
      installed(plugin, false);
      commit(preferences.edit().putBoolean("enabled:" + name(plugin), enabled));
    }
  }

  void remove(String plugin) {
    synchronized (LOCK) {
      name(plugin);
      commit(
          preferences
              .edit()
              .remove("current:" + plugin)
              .remove("previous:" + plugin)
              .remove("enabled:" + plugin));
    }
  }

  void rollback(String plugin) throws Exception {
    synchronized (LOCK) {
      name(plugin);
      String previous = preferences.getString("previous:" + plugin, "");
      if (previous.isEmpty()) throw new IllegalStateException("No previous version");
      verifiedInstallation(plugin, previous, true);
      commit(
          preferences.edit().putString("current:" + plugin, previous).remove("previous:" + plugin));
    }
  }

  // 仅在生命周期切换完成后回收未引用的版本，运行中的库不会被原地覆盖。
  void prune() {
    synchronized (LOCK) {
      Set<String> retained = new HashSet<>();
      for (Map.Entry<String, ?> entry : preferences.getAll().entrySet())
        if (entry.getKey().startsWith("current:") || entry.getKey().startsWith("previous:"))
          retained.add(entry.getValue().toString());
      File[] dirs = root.listFiles();
      if (dirs != null)
        for (File dir : dirs)
          if (dir.getName().matches("[0-9a-f]{64}") && !retained.contains(dir.getName()))
            clean(dir);
    }
  }

  private static void commit(SharedPreferences.Editor edit) {
    if (!edit.commit()) throw new IllegalStateException("Cannot save plugin activation");
  }

  private static void clean(File path) {
    try {
      if (android.system.OsConstants.S_ISDIR(android.system.Os.lstat(path.getPath()).st_mode)) {
        File[] children = path.listFiles();
        if (children != null) for (File child : children) clean(child);
      }
      path.delete();
    } catch (android.system.ErrnoException ignored) {
      /* 不跟随链接，也不处理已不存在的暂存文件。 */
    }
  }
}
