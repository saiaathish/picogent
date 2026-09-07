import crypto from "node:crypto";
import fs from "node:fs";
import fsp from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { once } from "node:events";
import { spawn } from "node:child_process";
import { chromium } from "playwright";

const candidateSHAInput = flag("--candidate-sha");
const fixtureBinary = flag("--binary");
const outputDirectory = flag("--out-dir");
const browserPath = flag("--browser-path") || process.env.PICOGENT_RENDERED_WINDOWS_BROWSER_PATH || "";

if (process.platform !== "win32") {
  fail("this collector must run on Windows");
}
if (!/^[0-9a-f]{40}$/i.test(candidateSHAInput)) {
  fail("--candidate-sha must be a full 40-character commit SHA");
}
const candidateSHA = candidateSHAInput.toLowerCase();
if (!fixtureBinary || !path.isAbsolute(fixtureBinary)) {
  fail("--binary must be an absolute fixture binary path");
}
if (!outputDirectory || !path.isAbsolute(outputDirectory)) {
  fail("--out-dir must be an absolute path outside the checkout");
}

const checkout = path.resolve(process.cwd());
const resolvedOutput = path.resolve(outputDirectory);
if (isInside(checkout, resolvedOutput)) {
  fail("--out-dir must be outside the source checkout");
}

let browser;
let seed;
let reload;

try {
  const realCheckout = await fsp.realpath(checkout);
  await assertOutputOutsideCheckout(realCheckout, resolvedOutput);
  await fsp.mkdir(resolvedOutput, { recursive: true });
  const realOutput = await fsp.realpath(resolvedOutput);
  if (isInside(realCheckout, realOutput)) {
    fail("--out-dir resolves inside the source checkout");
  }
  const fixtureRoot = await fsp.mkdtemp(path.join(os.tmpdir(), "picogent-rendered-windows-"));
  const fixtureHome = path.join(fixtureRoot, "home");
  const fixtureWorkspace = path.join(fixtureHome, "workspace");
  await fsp.mkdir(fixtureWorkspace, { recursive: true });

  const seedManifestPath = path.join(fixtureHome, "seed-manifest.json");
  const reloadManifestPath = path.join(fixtureHome, "reload-manifest.json");
  const fixtureEnvironment = {
    ...process.env,
    PICOGENT_NO_BROWSER: "1",
    PICOGENT_RENDERED_FIXTURE_ADDR: "127.0.0.1:0",
    PICOGENT_RENDERED_FIXTURE_HOME: fixtureHome,
    PICOGENT_RENDERED_FIXTURE_WORKSPACE: fixtureWorkspace,
    PICOGENT_RENDERED_FIXTURE_SOURCE_SHA: candidateSHA,
  };

  seed = startFixture("seed", seedManifestPath, fixtureEnvironment, fixtureHome, fixtureWorkspace);
  const seedURL = await seed.url;
  const seedManifestRecord = await readManifest(seedManifestPath);
  const seedManifest = seedManifestRecord.manifest;

  const launchOptions = { headless: true };
  if (browserPath) {
    launchOptions.executablePath = browserPath;
  } else {
    launchOptions.channel = "chrome";
  }
  browser = await chromium.launch(launchOptions);
  const browserVersion = browser.version();
  const browserID = `chrome-headless-${sanitizeIdentifier(browserVersion)}`;
  const browserContext = await browser.newContext();
  const page = await browserContext.newPage();

  await openFixture(page, seedURL);
  const seedInitialUndoDisabled = await page.locator("#undo-turn").isDisabled();
  if (!seedInitialUndoDisabled) {
    throw new Error("seed page exposed an undo control before a turn");
  }

  await page.locator("#prompt").fill("Apply the rendered recovery fixture mutation.");
  await page.locator("#send").click();
  await page.locator("#perm.is-on").waitFor({ state: "visible", timeout: 60000 });
  const permissionText = await page.locator("#perm-text").innerText();
  if (!permissionText.includes("rendered-recovery-probe.txt")) {
    throw new Error("permission prompt did not identify the recovery probe");
  }

  await page.locator('#perm [data-allow="1"]').click();
  await page.locator("#undo-turn:not([disabled])").waitFor({ state: "visible", timeout: 60000 });
  await waitForPageText(page, "Edited 1 file");
  await waitForPageText(page, "Changed files (1)");
  const probePath = path.join(fixtureWorkspace, "rendered-recovery-probe.txt");
  await waitForFile(probePath, true);
  const probeContent = await fsp.readFile(probePath, "utf8");
  if (probeContent !== "rendered recovery fixture\n") {
    throw new Error("allow mutation produced unexpected probe content");
  }

  await page.locator("#undo-turn").click();
  await waitFor(async () => (await page.locator("#turn-recovery").evaluate((element) => element.hidden)) === true);
  await waitForFile(probePath, false);
  if (!(await page.locator("#undo-turn").isDisabled())) {
    throw new Error("undo control remained enabled after restoration");
  }
  await stopFixture(seed.child);
  seed = null;

  reload = startFixture("reload", reloadManifestPath, fixtureEnvironment, fixtureHome, fixtureWorkspace);
  const reloadURL = await reload.url;
  const reloadManifestRecord = await readManifest(reloadManifestPath);
  const reloadManifest = reloadManifestRecord.manifest;
  await page.goto(reloadURL, { waitUntil: "domcontentloaded", timeout: 60000 });
  await page.locator("#prompt").waitFor({ state: "visible", timeout: 60000 });
  await waitForPageText(page, "Changed files (1)");
  await waitFor(async () => (await page.locator("#turn-recovery").evaluate((element) => element.hidden)) === true);
  await waitForFile(probePath, false);
  const reloadUndoDisabled = await page.locator("#undo-turn").isDisabled();
  if (!reloadUndoDisabled) {
    throw new Error("fresh reload exposed stale undo availability");
  }

  const screenshotPath = path.join(resolvedOutput, "windows-rendered-recovery.png");
  await page.screenshot({ path: screenshotPath, fullPage: true });
  await stopFixture(reload.child);
  reload = null;

  assertManifest(seedManifest, "seed");
  assertManifest(reloadManifest, "reload");
  await writeExclusive(path.join(resolvedOutput, "windows-seed-manifest.json"), seedManifestRecord.data);
  await writeExclusive(path.join(resolvedOutput, "windows-reload-manifest.json"), reloadManifestRecord.data);
  const observedAt = new Date().toISOString();
  const observation = {
    schema: "picogent.v4.rendered-recovery-observation.v1",
    candidate_sha: candidateSHA,
    platform: "windows",
    architecture: architectureForEvidence(),
    browser: browserID,
    fixture: "rendered-recovery",
    observed_at: observedAt,
    assertions: {
      seed_initial_undo_disabled: seedInitialUndoDisabled,
      permission_rendered: true,
      allow_rendered_contained_mutation: true,
      undo_rendered_restoration: true,
      reload_rendered_durable_history: true,
      reload_undo_disabled: reloadUndoDisabled,
      probe_absent_after_reload: !fs.existsSync(probePath),
      source_sha_verified: true,
      source_tree_modified: false,
    },
    provenance: {
      seed_manifest_sha256: digest(seedManifestRecord.data),
      reload_manifest_sha256: digest(reloadManifestRecord.data),
    },
  };
  const observationData = `${JSON.stringify(observation, null, 2)}\n`;
  const observationPath = path.join(resolvedOutput, "windows-rendered-recovery-observation.json");
  await writeExclusive(observationPath, observationData);
  const platformEvidence = {
    schema: "picogent.v4.rendered-platform-evidence.v1",
    candidate_sha: candidateSHA,
    platform: "windows",
    architecture: architectureForEvidence(),
    environment: "task-owned-disposable",
    browser: browserID,
    fixture: "rendered-recovery",
    observation_sha256: digest(Buffer.from(observationData)),
    screenshot_sha256: await digestFile(screenshotPath),
    observed_at: observedAt,
    verdict: "PASS",
    source_tree_modified: false,
  };
  const platformPath = path.join(resolvedOutput, "windows-rendered-platform.json");
  await writeExclusive(platformPath, `${JSON.stringify(platformEvidence, null, 2)}\n`);

  console.log(JSON.stringify({
    candidate_sha: candidateSHA,
    platform: platformEvidence.platform,
    architecture: platformEvidence.architecture,
    browser: platformEvidence.browser,
    observation_sha256: platformEvidence.observation_sha256,
    screenshot_sha256: platformEvidence.screenshot_sha256,
    platform_artifact_sha256: await digestFile(platformPath),
    verdict: platformEvidence.verdict,
  }));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
} finally {
  if (seed) await stopFixture(seed.child);
  if (reload) await stopFixture(reload.child);
  if (browser) await browser.close().catch(() => {});
}

function flag(name) {
  const index = process.argv.indexOf(name);
  return index >= 0 ? String(process.argv[index + 1] || "").trim() : "";
}

function fail(message) {
  throw new Error(message);
}

function isInside(parent, candidate) {
  const relative = path.relative(parent, candidate);
  return relative === "" || (relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative));
}

async function assertOutputOutsideCheckout(realCheckout, candidate) {
  const existingAncestor = await nearestExistingPath(candidate);
  const realAncestor = await fsp.realpath(existingAncestor);
  if (isInside(realCheckout, realAncestor)) {
    fail("--out-dir resolves inside the source checkout");
  }
}

async function nearestExistingPath(candidate) {
  let current = candidate;
  while (true) {
    try {
      await fsp.lstat(current);
      return current;
    } catch (error) {
      if (error?.code !== "ENOENT") throw error;
      const parent = path.dirname(current);
      if (parent === current) throw error;
      current = parent;
    }
  }
}

function sanitizeIdentifier(value) {
  const clean = String(value).replace(/[^a-z0-9.-]+/gi, "-").replace(/^-+|-+$/g, "");
  return clean.slice(0, 48) || "unknown";
}

function architectureForEvidence() {
  return process.arch === "arm64" ? "arm64" : "amd64";
}

function startFixture(phase, manifestPath, environment, home, workspace) {
  const child = spawn(fixtureBinary, [
    "-scenario", "recovery",
    "-phase", phase,
    "-home", home,
    "-workspace", workspace,
    "-manifest", manifestPath,
  ], { env: { ...environment, PICOGENT_RENDERED_FIXTURE_PHASE: phase, PICOGENT_RENDERED_FIXTURE_MANIFEST: manifestPath }, stdio: ["ignore", "pipe", "pipe"] });
  let output = "";
  let errorOutput = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => { output += chunk; });
  child.stderr.on("data", (chunk) => { errorOutput += chunk; });
  const url = new Promise((resolve, reject) => {
    const deadline = setTimeout(() => reject(new Error(`fixture ${phase} did not publish a URL`)), 60000);
    const onOutput = () => {
      const match = output.match(/url=(http:\/\/127\.0\.0\.1:\d+)/);
      if (match) {
        clearTimeout(deadline);
        resolve(match[1]);
      }
    };
    child.stdout.on("data", onOutput);
    child.once("error", (error) => {
      clearTimeout(deadline);
      reject(error);
    });
    child.once("exit", (code, signal) => {
      if (code !== null || signal !== null) {
        clearTimeout(deadline);
        reject(new Error(`fixture ${phase} exited before publishing a URL: code=${code} signal=${signal} stderr=${errorOutput}`));
      }
    });
    onOutput();
  });
  return { child, url };
}

async function openFixture(page, url) {
  await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60000 });
  await page.locator("#prompt").waitFor({ state: "visible", timeout: 60000 });
  await page.locator("#send:not([disabled])").waitFor({ state: "visible", timeout: 60000 });
}

async function waitForPageText(page, expected) {
  await waitFor(async () => (await page.locator("body").innerText()).includes(expected));
}

async function waitFor(predicate, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await delay(100);
  }
  throw new Error("timed out waiting for rendered recovery state");
}

async function waitForFile(filePath, shouldExist) {
  await waitFor(() => fs.existsSync(filePath) === shouldExist);
}

async function readManifest(manifestPath) {
  await waitForFile(manifestPath, true);
  const data = await fsp.readFile(manifestPath);
  return { manifest: JSON.parse(data.toString("utf8")), data };
}

function assertManifest(manifest, phase) {
  if (manifest.phase !== phase || manifest.source_sha !== candidateSHA || manifest.source_sha_verified !== true || manifest.source_tree_modified !== false) {
    throw new Error(`fixture ${phase} manifest did not prove the exact clean candidate SHA`);
  }
}

async function stopFixture(child) {
  if (!child || child.exitCode !== null) return;
  const exited = once(child, "exit").then(() => true);
  child.kill();
  if (await Promise.race([exited, delay(10000).then(() => false)])) return;
  if (child.exitCode !== null) return;
  child.kill("SIGKILL");
  if (!(await Promise.race([exited, delay(10000).then(() => false)])) && child.exitCode === null) {
    throw new Error("fixture process did not exit after termination");
  }
}

async function writeExclusive(filePath, data) {
  await fsp.writeFile(filePath, data, { encoding: "utf8", flag: "wx", mode: 0o600 });
}

async function digestFile(filePath) {
  return digest(await fsp.readFile(filePath));
}

function digest(data) {
  return crypto.createHash("sha256").update(data).digest("hex");
}

function delay(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}
