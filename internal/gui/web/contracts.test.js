"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const { createPrimaryEventDispatcher, mainPromptRequest, completionProofSummary } = require("./contracts.js");

function setupHarness(installResponse) {
  const elements = new Map();
  const makeElement = (id) => ({
    id,
    hidden: false,
    disabled: false,
    textContent: "",
    innerHTML: "",
    value: "",
    className: "",
    children: [],
    options: [{ value: "auto" }],
    classList: { toggle() {} },
    appendChild(child) { this.children.push(child); },
    addEventListener() {},
  });
  const getElement = (id) => {
    if (!elements.has(id)) elements.set(id, makeElement(id));
    return elements.get(id);
  };
  const dots = [0, 1, 2, 3].map((i) => makeElement(`dot-${i}`));
  const panels = [0, 1, 2, 3].map((i) => makeElement(`panel-${i}`));
  const calls = [];
  const setupStatus = {
    components: [
      { id: "home", ok: true, can_fix: false, detail: "ready" },
      { id: "git", ok: true, can_fix: false, detail: "ready" },
      { id: "codex-cli", ok: false, can_fix: true, detail: "missing" },
      { id: "claude-cli", ok: false, can_fix: true, detail: "missing" },
    ],
    logged_in: false,
  };
  const document = {
    getElementById: getElement,
    createElement: (tag) => makeElement(tag),
    querySelectorAll(selector) {
      if (selector === ".step-dot") return dots;
      if (selector === ".setup-stage") return panels;
      return [];
    },
  };
  const fetch = async (url, options) => {
    calls.push({ url, options });
    if (url === "/api/setup") {
      return { ok: true, json: async () => setupStatus };
    }
    if (url === "/api/setup/install") {
      if (typeof installResponse === "function") return installResponse();
      return { ok: true, json: async () => installResponse };
    }
    throw new Error(`unexpected setup request: ${url}`);
  };
  const context = {
    clearInterval,
    console,
    document,
    fetch,
    location: { origin: "http://picogent.test", href: "", search: "" },
    setInterval,
    URLSearchParams,
  };
  const script = fs.readFileSync(path.join(__dirname, "setup.js"), "utf8");
  vm.runInNewContext(script, context, { filename: "setup.js" });
  return { calls, elements, context };
}

async function settleSetup() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

test("dispatches the primary assistant, completion, and prompt-refresh events", () => {
  const calls = [];
  const dispatcher = createPrimaryEventDispatcher({
    assistantDelta: (text) => calls.push(["delta", text]),
    assistantFinal: (text) => calls.push(["final", text]),
    done: (event) => calls.push(["done", event.type]),
    promptsRefresh: (force, kind) => calls.push(["prompts", force, kind]),
  });

  for (const event of [
    { type: "assistant_delta", text: "Hello" },
    { type: "assistant_final", text: "Hello, world" },
    { type: "done" },
    { type: "prompts_refresh", text: "main" },
    { type: "prompts_refresh", text: "all" },
  ]) {
    assert.equal(dispatcher.dispatch(event), true, event.type);
  }

  assert.deepEqual(calls, [
    ["delta", "Hello"],
    ["final", "Hello, world"],
    ["done", "done"],
    ["prompts", true, "main"],
    ["prompts", true, "all"],
  ]);
});

test("normalizes empty payloads and ignores unrelated prompt refreshes", () => {
  const calls = [];
  const dispatcher = createPrimaryEventDispatcher({
    assistantDelta: (text) => calls.push(["delta", text]),
    assistantFinal: (text) => calls.push(["final", text]),
    promptsRefresh: (...args) => calls.push(["prompts", ...args]),
  });

  assert.equal(dispatcher.dispatch({ type: "assistant_delta" }), true);
  assert.equal(dispatcher.dispatch({ type: "assistant_final" }), true);
  assert.equal(dispatcher.dispatch({ type: "prompts_refresh", text: "side" }), true);
  assert.equal(dispatcher.dispatch({ type: "unknown" }), false);
  assert.deepEqual(calls, [["delta", ""], ["final", ""]]);
});

test("builds the deterministic main-prompt POST contract", () => {
  assert.deepEqual(mainPromptRequest(false), {
    url: "/api/prompts?kind=main",
    options: {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ kind: "main", refresh: false }),
    },
  });
  assert.equal(JSON.parse(mainPromptRequest(true).options.body).refresh, true);
});

test("summarizes durable completion proof without exposing evidence text", () => {
  assert.equal(completionProofSummary({ ready: true }), "Completion proof ready");
  assert.equal(completionProofSummary({
    ready: false,
    reason: "required criterion evidence is incomplete",
    missing_criteria: [0, 2],
    missing_requirements: ["tests"],
    verification_required: true,
    verification_current: false,
    evidence_summary: "secret tool output must not appear",
  }), "Completion proof pending: required criterion evidence is incomplete (2 required criteria missing, 1 quality requirement missing, workspace verification is not current)");
  assert.equal(completionProofSummary(null), "");
});

test("handles incomplete proof fallback and singular details", () => {
  assert.equal(completionProofSummary({
    ready: false,
    reason: "  ",
    missing_criteria: [1],
    missing_requirements: ["tests"],
    verification_required: true,
    verification_current: true,
  }), "Completion proof pending: completion proof is incomplete (1 required criterion missing, 1 quality requirement missing)");
  assert.equal(completionProofSummary({ ready: false }), "Completion proof pending: completion proof is incomplete");
  assert.equal(completionProofSummary("untrusted"), "");
});

test("setup only installs after an explicit button action", async () => {
  const harness = setupHarness({
    status: {
      components: [
        { id: "home", ok: true, can_fix: false, detail: "ready" },
        { id: "git", ok: true, can_fix: false, detail: "ready" },
        { id: "codex-cli", ok: true, can_fix: false, detail: "ready" },
        { id: "claude-cli", ok: true, can_fix: false, detail: "ready" },
      ],
      logged_in: false,
    },
  });
  await settleSetup();

  assert.deepEqual(harness.calls.map((call) => call.url), ["/api/setup"]);
  const installButton = harness.elements.get("install");
  assert.equal(typeof installButton.onclick, "function");

  await installButton.onclick();
  assert.deepEqual(harness.calls.map((call) => call.url), ["/api/setup", "/api/setup/install"]);
});

test("failed setup installation restores the explicit action", async () => {
  const harness = setupHarness(() => {
    throw new Error("network down");
  });
  await settleSetup();

  const installButton = harness.elements.get("install");
  await installButton.onclick();

  assert.equal(installButton.disabled, false);
  assert.equal(installButton.textContent, "Install missing pieces");
  assert.equal(harness.elements.get("stage-err").textContent, "network down");
});
