"use strict";

const assert = require("node:assert/strict");
const { execFile, spawn } = require("node:child_process");
const { once } = require("node:events");
const fs = require("node:fs/promises");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const { promisify } = require("node:util");
const { chromium } = require("playwright");

const run = promisify(execFile);

async function main() {
  const root = path.resolve(__dirname, "..");
  const port = await availablePort();
  const vault = await fs.mkdtemp(path.join(os.tmpdir(), "amber-desk-smoke-"));
  const executable = path.join(vault, process.platform === "win32" ? "amber-desk.exe" : "amber-desk");
  const go = process.env.GO_BINARY || (process.env.GOROOT ? path.join(process.env.GOROOT, "bin", process.platform === "win32" ? "go.exe" : "go") : process.platform === "win32" ? "C:\\Program Files\\Go\\bin\\go.exe" : "go");
  let server;
  let browser;
  try {
    await run(go, ["build", "-trimpath", "-o", executable, "."], { cwd: root });
    server = spawn(executable, [], {
      cwd: root,
      env: { ...process.env, ADDR: `127.0.0.1:${port}`, OBSIDIAN_VAULT: vault },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let serverLog = "";
    server.stdout.on("data", (data) => { serverLog += data; });
    server.stderr.on("data", (data) => { serverLog += data; });
    const baseURL = `http://127.0.0.1:${port}`;
    await waitForServer(`${baseURL}/api/health`, server, () => serverLog);

    browser = await chromium.launch({ headless: true });
    const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
    const failures = [];
    const localRequests = [];
    page.on("pageerror", (error) => failures.push(String(error)));
    page.on("console", (message) => { if (message.type() === "error") failures.push(message.text()); });
    page.on("request", (request) => {
      if (request.url().startsWith(baseURL)) localRequests.push(request.url());
      else failures.push(`external request: ${request.url()}`);
    });

    await page.goto(`${baseURL}/?lang=en`, { waitUntil: "networkidle" });
    await page.locator("#command-output").waitFor({ state: "visible" });
    await page.waitForTimeout(2_700);
    assert.equal(await page.locator("#command-output").textContent(), "WORKSPACE READY");
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1), false);

    const beforeMap = localRequests.length;
    await page.locator('[data-work-view="map-view"]').click();
    await page.locator("#world-map").waitFor({ state: "visible" });
    await page.waitForLoadState("networkidle");
    const initialMapRequests = localRequests.slice(beforeMap);
    assert.equal(initialMapRequests.some((url) => /roads-|regions\.geojson|urban\.geojson/.test(url)), false);
    const mapBox = await page.locator("#world-map").boundingBox();
    assert.ok(mapBox && mapBox.height > 300 && mapBox.width > 500);

    const roadResponse = page.waitForResponse((response) => response.url().endsWith("/data/roads-3.geojson"));
    await page.locator(".leaflet-control-zoom-in").click();
    await roadResponse;
    assert.deepEqual(failures, []);
    console.log("Browser smoke test passed");
  } finally {
    if (browser) await browser.close();
    if (server && server.exitCode === null) {
      server.kill();
      await Promise.race([once(server, "exit"), new Promise((resolve) => setTimeout(resolve, 3_000))]);
    }
    await fs.rm(vault, { recursive: true, force: true });
  }
}

async function availablePort() {
  const server = net.createServer();
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const { port } = server.address();
  server.close();
  await once(server, "close");
  return port;
}

async function waitForServer(url, server, log) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (server.exitCode !== null) throw new Error(`Amber Desk exited early:\n${log()}`);
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch (_) {
      // The Go process is still starting.
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`Amber Desk did not start:\n${log()}`);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
