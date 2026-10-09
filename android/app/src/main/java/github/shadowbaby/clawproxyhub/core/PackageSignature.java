package github.shadowbaby.clawproxyhub.core;

import java.io.ByteArrayInputStream;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.Signature;
import java.security.cert.CertificateFactory;

// 原生代码与完整界面均绑定 APP 证书，包名或来源地址不能授予信任。
final class PackageSignature {
    static PublicKey trustedKey(byte[] certificate, byte[][] trusted) throws Exception {
        boolean matches = false;
        for (byte[] signer : trusted) matches |= MessageDigest.isEqual(certificate, signer);
        if (!matches) throw new SecurityException("Package publisher is not trusted by this application");
        PublicKey key = CertificateFactory.getInstance("X.509").generateCertificate(new ByteArrayInputStream(certificate)).getPublicKey();
        if (!"RSA".equals(key.getAlgorithm())) throw new SecurityException("APP packages require an RSA certificate");
        return key;
    }

    static void verify(byte[] manifest, PublicKey key, byte[] signature) throws Exception {
        Signature verifier = Signature.getInstance("SHA256withRSA");
        verifier.initVerify(key); verifier.update(manifest);
        if (!verifier.verify(signature)) throw new SecurityException("Invalid package signature");
    }
}
