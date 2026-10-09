package github.shadowbaby.clawproxyhub.core;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.os.Build;
import android.util.Base64;
import java.security.PublicKey;
import org.json.JSONObject;

final class AppSignature {
    static byte[][] certificates(Context context) throws Exception {
        PackageInfo app = context.getPackageManager().getPackageInfo(context.getPackageName(),
                Build.VERSION.SDK_INT >= 28 ? PackageManager.GET_SIGNING_CERTIFICATES : PackageManager.GET_SIGNATURES);
        android.content.pm.Signature[] signers = Build.VERSION.SDK_INT >= 28 ? app.signingInfo.getApkContentsSigners() : app.signatures;
        byte[][] certificates = new byte[signers.length][];
        for (int i = 0; i < signers.length; i++) certificates[i] = signers[i].toByteArray();
        return certificates;
    }

    static PublicKey trustedKey(Context context, JSONObject signature) throws Exception {
        if (!"SHA256withRSA".equals(signature.getString("algorithm"))) throw new SecurityException("Unsupported APP package signature");
        return PackageSignature.trustedKey(Base64.decode(signature.getString("certificate"), Base64.DEFAULT), certificates(context));
    }
}
