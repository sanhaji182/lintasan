import forge from "node-forge";
import fs from "fs";
import path from "path";
import os from "os";

const MITM_DIR = path.join(os.homedir(), ".lintasan", "mitm");
const CA_KEY_PATH = path.join(MITM_DIR, "ca-key.pem");
const CA_CERT_PATH = path.join(MITM_DIR, "ca-cert.pem");

export function ensureMitmDir() {
  if (!fs.existsSync(MITM_DIR)) {
    fs.mkdirSync(MITM_DIR, { recursive: true });
  }
}

export function getCAPath() {
  return { key: CA_KEY_PATH, cert: CA_CERT_PATH, dir: MITM_DIR };
}

export function caExists() {
  return fs.existsSync(CA_KEY_PATH) && fs.existsSync(CA_CERT_PATH);
}

export function generateCA() {
  ensureMitmDir();

  const keys = forge.pki.rsa.generateKeyPair(2048);
  const cert = forge.pki.createCertificate();

  cert.publicKey = keys.publicKey;
  cert.serialNumber = "01";
  cert.validity.notBefore = new Date();
  cert.validity.notAfter = new Date();
  cert.validity.notAfter.setFullYear(cert.validity.notBefore.getFullYear() + 10);

  const attrs = [
    { name: "commonName", value: "Lintasan MITM CA" },
    { name: "organizationName", value: "Lintasan" },
    { name: "countryName", value: "ID" },
  ];

  cert.setSubject(attrs);
  cert.setIssuer(attrs);
  cert.setExtensions([
    { name: "basicConstraints", cA: true },
    { name: "keyUsage", keyCertSign: true, digitalSignature: true, cRLSign: true },
    { name: "subjectKeyIdentifier" },
  ]);

  cert.sign(keys.privateKey, forge.md.sha256.create());

  const pemKey = forge.pki.privateKeyToPem(keys.privateKey);
  const pemCert = forge.pki.certificateToPem(cert);

  fs.writeFileSync(CA_KEY_PATH, pemKey);
  fs.writeFileSync(CA_CERT_PATH, pemCert);

  return { key: pemKey, cert: pemCert };
}

export function loadCA() {
  if (!caExists()) {
    return generateCA();
  }
  return {
    key: fs.readFileSync(CA_KEY_PATH, "utf8"),
    cert: fs.readFileSync(CA_CERT_PATH, "utf8"),
  };
}

// Generate a domain-specific certificate signed by our CA
const certCache = new Map();

export function generateDomainCert(domain) {
  if (certCache.has(domain)) return certCache.get(domain);

  const ca = loadCA();
  const caKey = forge.pki.privateKeyFromPem(ca.key);
  const caCert = forge.pki.certificateFromPem(ca.cert);

  const keys = forge.pki.rsa.generateKeyPair(2048);
  const cert = forge.pki.createCertificate();

  cert.publicKey = keys.publicKey;
  cert.serialNumber = Date.now().toString(16);
  cert.validity.notBefore = new Date();
  cert.validity.notAfter = new Date();
  cert.validity.notAfter.setFullYear(cert.validity.notBefore.getFullYear() + 1);

  cert.setSubject([{ name: "commonName", value: domain }]);
  cert.setIssuer(caCert.subject.attributes);
  cert.setExtensions([
    { name: "basicConstraints", cA: false },
    { name: "keyUsage", digitalSignature: true, keyEncipherment: true },
    { name: "extKeyUsage", serverAuth: true },
    {
      name: "subjectAltName",
      altNames: [{ type: 2, value: domain }],
    },
  ]);

  cert.sign(caKey, forge.md.sha256.create());

  const result = {
    key: forge.pki.privateKeyToPem(keys.privateKey),
    cert: forge.pki.certificateToPem(cert),
  };

  certCache.set(domain, result);
  return result;
}
