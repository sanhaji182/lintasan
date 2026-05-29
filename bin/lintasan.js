#!/usr/bin/env node

import { spawn, execSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const DATA_DIR = path.join(ROOT, "data");
const DB_PATH = path.join(DATA_DIR, "lintasan.db");

const args = process.argv.slice(2);
const command = args[0] || "start";

const PORT = process.env.PORT || "20180";

function printBanner() {
  console.log("");
  console.log("  ╔═══════════════════════════════════════╗");
  console.log("  ║         L I N T A S A N               ║");
  console.log("  ║   Setiap Koneksi Punya Jalannya       ║");
  console.log("  ╚═══════════════════════════════════════╝");
  console.log("");
}

function printHelp() {
  printBanner();
  console.log("  Usage: lintasan [command]");
  console.log("");
  console.log("  Commands:");
  console.log("    start        Start the server (default)");
  console.log("    dev          Start in development mode");
  console.log("    setup        Initialize database");
  console.log("    build        Build for production");
  console.log("    status       Check if server is running");
  console.log("    mitm start   Start MITM bridge (intercept IDE traffic)");
  console.log("    mitm stop    Stop MITM bridge (restore hosts)");
  console.log("    mitm status  Check MITM bridge status");
  console.log("    mitm trust   Show CA cert install instructions");
  console.log("    help         Show this help message");
  console.log("");
  console.log("  Environment:");
  console.log("    PORT         Server port (default: 20180)");
  console.log("    DASHBOARD_PASSWORD  Dashboard login password (default: admin)");
  console.log("    LINTASAN_MASTER_KEY API key for proxy access");
  console.log("");
  console.log("  Quick Start:");
  console.log("    $ lintasan setup");
  console.log("    $ lintasan build");
  console.log("    $ lintasan");
  console.log("");
  console.log("  MITM Bridge (intercept Copilot/Cursor/Kiro):");
  console.log("    $ lintasan mitm trust    # Install CA cert (one-time)");
  console.log("    $ sudo lintasan mitm start");
  console.log("");
  console.log("  Dashboard: http://localhost:" + PORT + "/dashboard");
  console.log("");
}

function setup() {
  printBanner();
  console.log("  ⚙  Initializing database...\n");

  if (!fs.existsSync(DATA_DIR)) {
    fs.mkdirSync(DATA_DIR, { recursive: true });
  }

  try {
    execSync("node scripts/seed.js", { cwd: ROOT, stdio: "inherit" });
    console.log("\n  ✅ Setup complete!");
    console.log(`  📂 Database: ${DB_PATH}`);
    console.log(`\n  Next: run 'lintasan build' then 'lintasan'\n`);
  } catch (e) {
    console.error("  ❌ Setup failed:", e.message);
    process.exit(1);
  }
}

function build() {
  printBanner();
  console.log("  🔨 Building for production...\n");

  try {
    execSync("npx next build", { cwd: ROOT, stdio: "inherit", env: { ...process.env, PORT } });
    console.log("\n  ✅ Build complete!");
    console.log(`\n  Next: run 'lintasan' to start the server\n`);
  } catch (e) {
    console.error("  ❌ Build failed:", e.message);
    process.exit(1);
  }
}

function start() {
  // Auto-setup if DB doesn't exist
  if (!fs.existsSync(DB_PATH)) {
    console.log("  📦 First run detected — initializing database...\n");
    setup();
    console.log("");
  }

  // Check if build exists
  const nextDir = path.join(ROOT, ".next");
  if (!fs.existsSync(nextDir)) {
    console.log("  🔨 No build found — building first...\n");
    build();
    console.log("");
  }

  printBanner();
  console.log(`  🚀 Starting Lintasan on port ${PORT}`);
  console.log(`  📊 Dashboard: http://localhost:${PORT}/dashboard`);
  console.log(`  📡 API: http://localhost:${PORT}/api/v1`);
  console.log("");

  const server = spawn("npx", ["next", "start", "-p", PORT], {
    cwd: ROOT,
    stdio: "inherit",
    env: { ...process.env, PORT },
  });

  server.on("error", (err) => {
    console.error("  ❌ Failed to start:", err.message);
    process.exit(1);
  });

  server.on("close", (code) => {
    process.exit(code || 0);
  });

  // Graceful shutdown
  process.on("SIGINT", () => { server.kill("SIGINT"); });
  process.on("SIGTERM", () => { server.kill("SIGTERM"); });
}

function dev() {
  // Auto-setup if DB doesn't exist
  if (!fs.existsSync(DB_PATH)) {
    setup();
    console.log("");
  }

  printBanner();
  console.log(`  🔧 Starting Lintasan in dev mode on port ${PORT}`);
  console.log(`  📊 Dashboard: http://localhost:${PORT}/dashboard`);
  console.log("");

  const server = spawn("npx", ["next", "dev", "-p", PORT], {
    cwd: ROOT,
    stdio: "inherit",
    env: { ...process.env, PORT },
  });

  server.on("close", (code) => { process.exit(code || 0); });
  process.on("SIGINT", () => { server.kill("SIGINT"); });
  process.on("SIGTERM", () => { server.kill("SIGTERM"); });
}

function status() {
  try {
    const res = execSync(`curl -s http://127.0.0.1:${PORT}/api/auth/check`, { timeout: 5000 });
    const data = JSON.parse(res.toString());
    console.log(`  ✅ Lintasan is running on port ${PORT}`);
  } catch {
    console.log(`  ❌ Lintasan is not running on port ${PORT}`);
    process.exit(1);
  }
}

async function mitm(subcommand) {
  const { loadCA, generateCA, caExists, getCAPath } = await import("../lib/mitm/ca.js");
  const { addHostsEntries, removeHostsEntries, isHostsConfigured, TARGET_HOSTS } = await import("../lib/mitm/dns.js");

  switch (subcommand) {
    case "start": {
      printBanner();
      console.log("  🔒 Starting MITM Bridge...\n");

      // Ensure CA exists
      if (!caExists()) {
        console.log("  🔑 Generating Root CA certificate...");
        generateCA();
        const { dir } = getCAPath();
        console.log(`  📂 CA stored at: ${dir}`);
        console.log("  ⚠  Trust the CA cert first: lintasan mitm trust\n");
      }

      const { startMitmServer } = await import("../lib/mitm/server.js");
      startMitmServer();
      break;
    }

    case "stop": {
      printBanner();
      console.log("  🛑 Stopping MITM Bridge...\n");
      removeHostsEntries();
      console.log("  ✅ MITM Bridge stopped — hosts file restored");
      break;
    }

    case "status": {
      const configured = isHostsConfigured();
      if (configured) {
        console.log("  ✅ MITM Bridge is active");
        console.log(`  📡 Intercepting: ${TARGET_HOSTS.join(", ")}`);
      } else {
        console.log("  ❌ MITM Bridge is not active");
      }
      break;
    }

    case "trust": {
      if (!caExists()) {
        console.log("  🔑 Generating Root CA certificate...");
        generateCA();
      }
      const { cert, dir } = getCAPath();
      printBanner();
      console.log("  🔐 Trust the Lintasan CA certificate\n");
      console.log(`  CA cert location: ${cert}\n`);
      console.log("  macOS:");
      console.log(`    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ${cert}\n`);
      console.log("  Linux (Ubuntu/Debian):");
      console.log(`    sudo cp ${cert} /usr/local/share/ca-certificates/lintasan-ca.crt`);
      console.log("    sudo update-ca-certificates\n");
      console.log("  Windows:");
      console.log(`    certutil -addstore -f "ROOT" ${cert}\n`);
      console.log("  Node.js (env var):");
      console.log(`    export NODE_EXTRA_CA_CERTS=${cert}\n`);
      break;
    }

    default:
      console.error(`  Unknown mitm command: ${subcommand}`);
      console.log("  Available: start, stop, status, trust");
      process.exit(1);
  }
}

// Route commands
switch (command) {
  case "start":
    start();
    break;
  case "dev":
    dev();
    break;
  case "setup":
    setup();
    break;
  case "build":
    build();
    break;
  case "status":
    status();
    break;
  case "mitm":
    mitm(args[1] || "start");
    break;
  case "help":
  case "--help":
  case "-h":
    printHelp();
    break;
  default:
    console.error(`  Unknown command: ${command}`);
    printHelp();
    process.exit(1);
}
