import https from "https";
import http from "http";
import tls from "tls";
import { generateDomainCert, loadCA } from "./ca.js";
import { TARGET_HOSTS, resolveHost, addHostsEntries, removeHostsEntries } from "./dns.js";
import { interceptRequest } from "./interceptor.js";

const LINTASAN_PORT = process.env.PORT || 20180;
const MITM_PORT = parseInt(process.env.MITM_PORT || "8443");
let server = null;

export function startMitmServer() {
  const ca = loadCA();

  server = https.createServer(
    {
      SNICallback: (domain, cb) => {
        const { key, cert } = generateDomainCert(domain);
        const ctx = tls.createSecureContext({ key, cert, ca: ca.cert });
        cb(null, ctx);
      },
      // Fallback cert
      key: generateDomainCert("localhost").key,
      cert: generateDomainCert("localhost").cert,
    },
    async (req, res) => {
      const host = req.headers.host?.split(":")[0];

      // Prevent loops
      if (req.headers["x-lintasan-mitm"]) {
        return proxyToUpstream(req, res, host);
      }

      // Check if this is a target host
      if (!TARGET_HOSTS.includes(host)) {
        return proxyToUpstream(req, res, host);
      }

      // Try to intercept
      try {
        const intercepted = await interceptRequest(req, res, host);
        if (!intercepted) {
          // Not interceptable — pass through to real upstream
          await proxyToUpstream(req, res, host);
        }
      } catch (err) {
        console.error(`  ❌ MITM error [${host}]:`, err.message);
        await proxyToUpstream(req, res, host);
      }
    }
  );

  // Add hosts entries
  addHostsEntries();

  const listenPort = MITM_PORT || (process.getuid?.() === 0 ? 443 : 8443);

  server.listen(listenPort, "0.0.0.0", () => {
    console.log(`  🔒 MITM Bridge listening on :${listenPort}`);
    console.log(`  📡 Intercepting: ${TARGET_HOSTS.length} hosts`);
    console.log(`  🔀 Routing through Lintasan :${LINTASAN_PORT}`);
    if (listenPort !== 443) {
      console.log(`\n  ⚠  Running on port ${listenPort} (not 443).`);
      console.log(`     IDEs won't auto-route here unless you use iptables redirect:`);
      console.log(`     sudo iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port ${listenPort}`);
    }
  });

  server.on("error", (err) => {
    if (err.code === "EACCES") {
      console.error("  ❌ Port 443 requires root/sudo. Run: sudo lintasan mitm start");
    } else if (err.code === "EADDRINUSE") {
      console.error("  ❌ Port 443 already in use. Stop other HTTPS servers first.");
    } else {
      console.error("  ❌ MITM server error:", err.message);
    }
    process.exit(1);
  });

  // Graceful cleanup
  const cleanup = () => {
    console.log("\n  🧹 Cleaning up MITM...");
    removeHostsEntries();
    if (server) server.close();
    process.exit(0);
  };

  process.on("SIGINT", cleanup);
  process.on("SIGTERM", cleanup);
  process.on("uncaughtException", (err) => {
    console.error("  ❌ Uncaught:", err.message);
    cleanup();
  });
}

export function stopMitmServer() {
  removeHostsEntries();
  if (server) {
    server.close();
    server = null;
  }
  console.log("  ✅ MITM Bridge stopped");
}

// Proxy request to real upstream (bypass DNS hijack via resolved IP)
async function proxyToUpstream(req, res, host) {
  try {
    const ip = await resolveHost(host);
    const options = {
      hostname: ip,
      port: 443,
      path: req.url,
      method: req.method,
      headers: {
        ...req.headers,
        host: host, // Keep original host header
      },
      rejectUnauthorized: true,
      servername: host, // SNI for upstream TLS
    };

    const proxyReq = https.request(options, (proxyRes) => {
      res.writeHead(proxyRes.statusCode, proxyRes.headers);
      proxyRes.pipe(res);
    });

    proxyReq.on("error", (err) => {
      console.error(`  ⚠  Upstream error [${host}]:`, err.message);
      if (!res.headersSent) {
        res.writeHead(502);
        res.end(`MITM upstream error: ${err.message}`);
      }
    });

    req.pipe(proxyReq);
  } catch (err) {
    if (!res.headersSent) {
      res.writeHead(502);
      res.end(`DNS resolve error: ${err.message}`);
    }
  }
}
