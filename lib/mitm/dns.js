import fs from "fs";
import { execSync } from "child_process";
import os from "os";

const HOSTS_PATH = "/etc/hosts";
const MARKER_START = "# --- LINTASAN MITM START ---";
const MARKER_END = "# --- LINTASAN MITM END ---";

// Target hosts to intercept
export const TARGET_HOSTS = [
  "api.individual.githubcopilot.com",   // GitHub Copilot
  "q.us-east-1.amazonaws.com",          // Kiro AI
  "api2.cursor.sh",                     // Cursor
  "cloudcode-pa.googleapis.com",        // Google IDX / Gemini
  "daily-cloudcode-pa.googleapis.com",  // Google IDX (daily)
];

export function addHostsEntries() {
  try {
    const content = fs.readFileSync(HOSTS_PATH, "utf8");

    // Already added?
    if (content.includes(MARKER_START)) {
      console.log("  ℹ  Hosts entries already present");
      return true;
    }

    const entries = TARGET_HOSTS.map((h) => `127.0.0.1 ${h}`).join("\n");
    const block = `\n${MARKER_START}\n${entries}\n${MARKER_END}\n`;

    fs.appendFileSync(HOSTS_PATH, block);
    flushDNS();
    console.log(`  ✅ Added ${TARGET_HOSTS.length} hosts entries`);
    return true;
  } catch (err) {
    if (err.code === "EACCES") {
      console.log("  ⚠  Cannot modify /etc/hosts (no sudo). Run with sudo or add manually:");
      console.log(`     sudo lintasan mitm start`);
      console.log("");
      console.log("  Or add these lines to /etc/hosts manually:");
      TARGET_HOSTS.forEach((h) => console.log(`     127.0.0.1 ${h}`));
      return false;
    }
    throw err;
  }
}

export function removeHostsEntries() {
  const content = fs.readFileSync(HOSTS_PATH, "utf8");

  if (!content.includes(MARKER_START)) {
    return;
  }

  const regex = new RegExp(
    `\\n?${escapeRegex(MARKER_START)}[\\s\\S]*?${escapeRegex(MARKER_END)}\\n?`,
    "g"
  );
  const cleaned = content.replace(regex, "\n");

  fs.writeFileSync(HOSTS_PATH, cleaned);
  flushDNS();
  console.log("  ✅ Removed hosts entries");
}

export function isHostsConfigured() {
  const content = fs.readFileSync(HOSTS_PATH, "utf8");
  return content.includes(MARKER_START);
}

function flushDNS() {
  const platform = os.platform();
  try {
    if (platform === "darwin") {
      execSync("dscacheutil -flushcache && sudo killall -HUP mDNSResponder", { stdio: "ignore" });
    } else if (platform === "linux") {
      execSync("systemd-resolve --flush-caches 2>/dev/null || resolvectl flush-caches 2>/dev/null || true", { stdio: "ignore" });
    }
  } catch {
    // DNS flush is best-effort
  }
}

function escapeRegex(str) {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Resolve real IP via external DNS (bypass hosts hijack)
import { Resolver } from "dns";
const resolver = new Resolver();
resolver.setServers(["8.8.8.8", "1.1.1.1"]);

const dnsCache = new Map();
const DNS_TTL = 5 * 60 * 1000; // 5 minutes

export function resolveHost(hostname) {
  return new Promise((resolve, reject) => {
    const cached = dnsCache.get(hostname);
    if (cached && Date.now() - cached.time < DNS_TTL) {
      return resolve(cached.ip);
    }

    resolver.resolve4(hostname, (err, addresses) => {
      if (err) return reject(err);
      const ip = addresses[0];
      dnsCache.set(hostname, { ip, time: Date.now() });
      resolve(ip);
    });
  });
}
