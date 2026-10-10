package github.shadowbaby.clawproxyhub.core;

import java.io.File;
import java.nio.file.Files;
import java.security.MessageDigest;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;
import static org.junit.Assert.*;

public class ExtensionLibraryTest {
    @Rule public TemporaryFolder temporary = new TemporaryFolder();

    private String digest(byte[] bytes) throws Exception {
        StringBuilder value = new StringBuilder();
        for (byte b : MessageDigest.getInstance("SHA-256").digest(bytes)) value.append(String.format("%02x", b & 255));
        return value.toString();
    }

    @Test public void verifiesInstalledIdentityDigestAbiAndMinimumSdk() throws Exception {
        File root = temporary.newFolder("versions");
        File file = new File(root, "lua-editor/" + "a".repeat(64) + "/backend/android-arm64/libservice.so");
        assertTrue(file.getParentFile().mkdirs());
        byte[] bytes = new byte[64];
        bytes[0] = 127; bytes[1] = 'E'; bytes[2] = 'L'; bytes[3] = 'F'; bytes[4] = 2; bytes[5] = 1; bytes[16] = 3; bytes[18] = (byte) 183;
        Files.write(file.toPath(), bytes);
        String hash = digest(bytes);
        String[] abis = {"arm64-v8a"};
        assertEquals(file.getCanonicalFile(), ExtensionLibrary.verify(root, file.getPath(), hash, 24, 35, abis));
        assertThrows(SecurityException.class, () -> ExtensionLibrary.verify(root, file.getPath(), "b".repeat(64), 24, 35, abis));
        assertThrows(SecurityException.class, () -> ExtensionLibrary.verify(root, file.getPath(), hash, 36, 35, abis));
        assertThrows(SecurityException.class, () -> ExtensionLibrary.verify(root, file.getPath(), hash, 24, 35, new String[]{"x86_64"}));
        File outside = temporary.newFile("outside.so");
        Files.write(outside.toPath(), bytes);
        assertThrows(SecurityException.class, () -> ExtensionLibrary.verify(root, outside.getPath(), hash, 24, 35, abis));
        bytes[16] = 2;
        Files.write(file.toPath(), bytes);
        assertThrows(SecurityException.class, () -> ExtensionLibrary.verify(root, file.getPath(), digest(bytes), 24, 35, abis));
    }

    @Test public void acceptsSdkIdentifiersAndRejectsMalformedIdentifiers() throws Exception {
        File root = temporary.newFolder("identifiers");
        byte[] bytes = new byte[64];
        bytes[0] = 127; bytes[1] = 'E'; bytes[2] = 'L'; bytes[3] = 'F'; bytes[4] = 2; bytes[5] = 1; bytes[16] = 3; bytes[18] = 62;
        String hash = digest(bytes);
        String[] abis = {"x86_64"};
        for (String id : new String[]{"example.editor_v2", "a".repeat(96), "lua-editor"}) {
            File file = new File(root, id + "/" + "a".repeat(64) + "/libservice.so");
            assertTrue(file.getParentFile().mkdirs());
            Files.write(file.toPath(), bytes);
            assertEquals(file.getCanonicalFile(), ExtensionLibrary.verify(root, file.getPath(), hash, 24, 35, abis));
        }
        for (String id : new String[]{"Example", "a".repeat(97), "editor..name", "editor-", "_editor"}) {
            File file = new File(root, id + "/" + "a".repeat(64) + "/libservice.so");
            assertTrue(file.getParentFile().mkdirs());
            Files.write(file.toPath(), bytes);
            assertThrows(id, SecurityException.class, () -> ExtensionLibrary.verify(root, file.getPath(), hash, 24, 35, abis));
        }
    }
}
