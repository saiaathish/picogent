"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const {
  createPrimaryEventDispatcher,
  createPermissionResponseController,
  mainPromptRequest,
  completionProofSummary,
  contradictionSummary,
} = require("./contracts.js");

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function permissionPrompt(id, extra = {}) {
  return { permission_id: id, summary: "Change " + id, hint: "Review this change.", ...extra };
}

// Exercise the real app wiring without booting unrelated UI or making requests.
function permissionAppHarness(send = () => ({ status: 204 })) {
  const script = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
  const section = (start, end) => {
    const from = script.indexOf(start);
    const to = script.indexOf(end, from);
    assert.ok(from >= 0 && to > from, "app section: " + start);
    return script.slice(from, to);
  };
  const elements = new Map();
  const element = (id) => {
    if (!elements.has(id)) {
      const classes = new Set();
      elements.set(id, {
        dataset: {}, hidden: true, disabled: false, textContent: "", children: [],
        classList: {
          add: (name) => classes.add(name), remove: (name) => classes.delete(name),
          contains: (name) => classes.has(name), toggle() {},
        },
        querySelector: () => null, querySelectorAll: () => [],
        addEventListener(type, listener) { this[type] = listener; },
      });
    }
    return elements.get(id);
  };
  const buttons = [
    { dataset: { allow: "0" }, disabled: false },
    { dataset: { turn: "1" }, disabled: false },
    { dataset: { always: "1" }, disabled: false },
    { dataset: { allow: "1" }, disabled: false },
  ];
  const permEl = element("perm");
  permEl.querySelectorAll = () => buttons;
  const calls = [];
  const snapshots = [];
  const noop = () => {};
  const context = {
    window: { PicogentWebContracts: { createPrimaryEventDispatcher, createPermissionResponseController } },
    $: element, permEl, permText: element("perm-text"), permHint: element("perm-hint"),
    permTitle: element("perm-title"), modeSeg: element("mode-seg"), logEl: element("log"),
    sendBtn: element("send"), reasoningEl: element("reasoning"),
    viewEpoch: 0, refreshGeneration: 0, sessionId: "session", busy: true, ready: true,
    chatRequestsPending: 0, historyReplayPending: false, userModelChoice: "auto",
    stream: null, activityPanel: null,
    fetch(url, options) {
      if (url === "/api/permission") {
        const call = { url, options, payload: JSON.parse(options.body) };
        calls.push(call);
        return send(call);
      }
      assert.equal(url, "/api/state");
      const request = deferred();
      snapshots.push(request);
      return request.promise.then((state) => ({ json: async () => state }));
    },
    renderTopContext: noop, renderTaskMode: noop, fillModelPick: noop, renderAuthBanner: noop,
    renderOverview: noop, renderContext: noop, renderTaskProgress: noop, replayMessages: noop,
    setUndoAvailable: noop, syncUndoControl: noop, setThinking: noop, syncEmpty: noop,
    loadThreads: async () => {}, loadProjects: async () => {}, updateActivityPanel: noop,
    appendAssistantDelta: noop, typeAssistantFull: noop, loadHeroPrompts: noop, refreshActivity: noop,
  };
  vm.runInNewContext([
    section("const permissionResponses = ", "\n/* ─── Extensions finder"),
    section("function finishTurnUI()", "\n/* ─── SSE"),
    section("const primaryEventDispatcher = ", "\nfunction verificationPresentation"),
    section("function renderPermission(state)", "\nfunction connectEvents()"),
    section("async function refresh(", "\nlet authPollTimer"),
    "const appRefresh = refresh; const refreshes = [];",
    "refresh = (...args) => { const pending = appRefresh(...args); refreshes.push(pending); return pending; };",
    "globalThis.permissionApp = { controller: permissionResponses, showPermission, refresh, refreshes, dispatch: (event) => primaryEventDispatcher.dispatch(event) };",
  ].join("\n"), context, { filename: "app-permission-wiring.js" });
  return {
    ...context.permissionApp, calls, snapshots,
    click: (index) => permEl.click({ target: { closest: () => buttons[index] } }),
    view: () => ({
      visible: permEl.classList.contains("is-on"), id: permEl.dataset.permissionId || "",
      text: context.permText.textContent, hint: context.permHint.textContent,
      hintHidden: context.permHint.hidden, disabled: buttons.map((button) => button.disabled),
    }),
  };
}

for (const [index, name, choice] of [
  [0, "deny", { allow: false, turn: false, always: false }],
  [1, "this turn", { allow: false, turn: true, always: false }],
  [2, "always allow", { allow: false, turn: false, always: true }],
  [3, "allow", { allow: true, turn: false, always: false }],
]) {
  test("permission app sends the exact " + name + " payload and hides only on 204", async () => {
    const app = permissionAppHarness();
    app.showPermission(permissionPrompt("17"));
    await app.click(index);
    assert.equal(app.calls.length, 1);
    assert.equal(app.calls[0].options.method, "POST");
    assert.equal(app.calls[0].options.headers["content-type"], "application/json");
    assert.deepEqual(app.calls[0].payload, { ...choice, permission_id: "17" });
    assert.equal(app.view().visible, false);
    assert.equal(app.view().id, "");
  });
}

for (const status of [200, 201, 202, 400, 401, 403, 404, 409, 410, 500, 503]) {
  test("permission HTTP " + status + " preserves the prompt and permits retry", async () => {
    let count = 0;
    const app = permissionAppHarness(() => ({ status: count++ === 0 ? status : 204, ok: true }));
    app.showPermission(permissionPrompt("17"));
    await app.click(3);
    assert.equal(app.view().visible, true);
    assert.equal(app.view().id, "17");
    assert.deepEqual(app.view().disabled, [false, false, false, false]);
    assert.match(app.view().hint, /Review this change\./);
    assert.ok(app.view().hint.includes("HTTP " + status));
    assert.match(app.view().hint, /try again/);
    const error = app.controller.getState().error;
    app.showPermission(permissionPrompt("17", { hint: "Updated guidance." }));
    assert.equal(app.controller.getState().error, error);
    assert.ok(app.view().hint.includes("Updated guidance. " + error));
    await app.click(3);
    assert.equal(app.calls.length, 2);
    assert.deepEqual(app.calls[1].payload, app.calls[0].payload);
    assert.equal(app.view().visible, false);
  });
}

for (const synchronous of [false, true]) {
  test("permission " + (synchronous ? "synchronous" : "network") + " failure preserves choices and retry", async () => {
    let count = 0;
    const app = permissionAppHarness(() => {
      if (count++ > 0) return { status: 204 };
      if (synchronous) throw new Error("transport unavailable");
      return Promise.reject(new Error("network down"));
    });
    app.showPermission(permissionPrompt("17"));
    await app.click(0);
    assert.equal(app.view().visible, true);
    assert.deepEqual(app.view().disabled, [false, false, false, false]);
    assert.match(app.view().hint, /Review this change\. .*try again/);
    await app.click(0);
    assert.equal(app.calls.length, 2);
    assert.equal(app.view().visible, false);
  });
}

test("permission pending disables all choices synchronously, deduplicates, and survives same-ID redraw", async () => {
  const response = deferred();
  const app = permissionAppHarness(() => response.promise);
  app.showPermission(permissionPrompt("17"));
  const first = app.click(3);
  assert.equal(app.calls.length, 1);
  assert.deepEqual(app.view().disabled, [true, true, true, true]);
  for (let i = 0; i < 4; i++) await app.click(i);
  assert.equal(await app.controller.respond("17", { allow: false }), false);
  assert.equal(app.calls.length, 1);
  app.showPermission(permissionPrompt("17", { summary: "Redrawn", hint: "New hint." }));
  assert.equal(app.view().text, "Redrawn");
  assert.equal(app.view().hint, "New hint.");
  assert.deepEqual(app.view().disabled, [true, true, true, true]);
  response.resolve({ status: 204 });
  await first;
  assert.equal(app.view().visible, false);
});

for (const result of ["success", "http failure", "network failure"]) {
  for (const replacement of ["enabled", "pending", "retry error"]) {
    test("old permission " + result + "/finally cannot change a replacement with " + replacement + " choices", async () => {
      const old = deferred();
      const next = deferred();
      const app = permissionAppHarness(({ payload }) => payload.permission_id === "17" ? old.promise : next.promise);
      app.showPermission(permissionPrompt("17"));
      const oldClick = app.click(3);
      app.showPermission(permissionPrompt("18", { hint: "Replacement guidance." }));
      assert.deepEqual(app.view().disabled, [false, false, false, false]);
      let nextClick = replacement === "enabled" ? null : app.click(1);
      if (replacement === "retry error") {
        next.resolve({ status: 503 });
        await nextClick;
        nextClick = null;
        assert.match(app.view().hint, /HTTP 503.*try again/);
      }
      const before = app.view();
      const revision = app.controller.getRevision();
      if (result === "network failure") old.reject(new Error("old network failure"));
      else old.resolve({ status: result === "success" ? 204 : 409 });
      await oldClick;
      assert.deepEqual(app.view(), before);
      assert.equal(app.controller.getRevision(), revision);
      if (nextClick) {
        next.resolve({ status: 204 });
        await nextClick;
        assert.equal(app.view().visible, false);
      }
    });
  }
}

test("permission ownership rejects missing/mismatched IDs and an old attempt after ID reuse", async () => {
  const old = deferred();
  const app = permissionAppHarness(() => old.promise);
  app.showPermission({ summary: "No ID" });
  assert.equal(await app.controller.respond("", { allow: true }), false);
  app.showPermission(permissionPrompt("17"));
  assert.equal(await app.controller.respond("18", { allow: true }), false);
  assert.equal(app.calls.length, 0);
  const click = app.click(3);
  app.showPermission(permissionPrompt("18"));
  app.showPermission(permissionPrompt("17", { hint: "Reused ID." }));
  const before = app.view();
  old.resolve({ status: 204 });
  await click;
  assert.deepEqual(app.view(), before);
});

test("a snapshot fetched before acknowledgement cannot revive it; live replay is blocked until fresh authority", async () => {
  const response = deferred();
  const app = permissionAppHarness(() => response.promise);
  app.showPermission(permissionPrompt("17"));
  const click = app.click(3);
  const stale = app.refresh();
  response.resolve({ status: 204 });
  await click;
  app.snapshots[0].resolve({ busy: true, pending_perm: permissionPrompt("17") });
  await stale;
  assert.equal(app.view().visible, false);
  app.showPermission(permissionPrompt("17"));
  assert.equal(app.view().visible, false);
  app.showPermission(permissionPrompt("18"));
  app.showPermission(permissionPrompt("17", { hint: "Replayed old event." }));
  assert.equal(app.view().id, "18");
  const fresh = app.refresh();
  app.snapshots[1].resolve({ busy: true, pending_perm: permissionPrompt("17", { hint: "New server." }) });
  await fresh;
  assert.equal(app.view().id, "17");
  assert.equal(app.view().hint, "New server.");
  assert.deepEqual(app.view().disabled, [false, false, false, false]);
  app.showPermission(permissionPrompt("17", { hint: "Fresh live redraw." }));
  assert.equal(app.view().hint, "Fresh live redraw.");
});

test("one acknowledgement high-water mark blocks older replays without rounding uint64 IDs", async () => {
  const app = permissionAppHarness();
  const first = "9007199254740992";
  const second = "9007199254740993";
  app.showPermission(permissionPrompt(first));
  await app.click(3);
  app.showPermission(permissionPrompt(second));
  assert.equal(app.view().id, second, "a larger ID must remain actionable");
  await app.click(3);
  app.showPermission(permissionPrompt(first));
  assert.equal(app.view().visible, false, "older acknowledged IDs stay suppressed");
  app.showPermission(permissionPrompt("9007199254740994"));
  app.showPermission(permissionPrompt(second));
  assert.equal(app.view().id, "9007199254740994");
  const restarted = app.refresh();
  app.snapshots[0].resolve({ busy: true, pending_perm: permissionPrompt("1", { hint: "Restarted server." }) });
  await restarted;
  assert.equal(app.view().id, "1");
  assert.deepEqual(app.view().disabled, [false, false, false, false]);
  app.showPermission(permissionPrompt("1", { hint: "Restart redraw." }));
  assert.equal(app.view().hint, "Restart redraw.");
});

test("pre-event snapshots cannot erase replacements or overwrite a same-ID redraw", async () => {
  const app = permissionAppHarness();
  app.showPermission(permissionPrompt("17"));
  const empty = app.refresh();
  app.showPermission(permissionPrompt("18"));
  app.snapshots[0].resolve({ busy: false });
  await empty;
  assert.equal(app.view().id, "18");
  const stale = app.refresh();
  app.showPermission(permissionPrompt("18", { hint: "Latest live guidance." }));
  app.snapshots[1].resolve({ busy: true, pending_perm: permissionPrompt("18", { hint: "Old guidance." }) });
  await stale;
  assert.equal(app.view().hint, "Latest live guidance.");
});

test("snapshots predating a click or failed completion cannot erase pending/error state", async () => {
  const response = deferred();
  const app = permissionAppHarness(() => response.promise);
  app.showPermission(permissionPrompt("17"));
  const beforeClick = app.refresh();
  const click = app.click(3);
  app.snapshots[0].resolve({ busy: false });
  await beforeClick;
  assert.equal(app.view().visible, true);
  assert.deepEqual(app.view().disabled, [true, true, true, true]);
  const beforeFailure = app.refresh();
  response.resolve({ status: 503 });
  await click;
  const before = app.view();
  app.snapshots[1].resolve({ busy: false });
  await beforeFailure;
  assert.deepEqual(app.view(), before);
});

test("authoritative same-ID snapshots preserve both pending and retry error", async () => {
  const response = deferred();
  const app = permissionAppHarness(() => response.promise);
  app.showPermission(permissionPrompt("17"));
  const click = app.click(3);
  const pending = app.refresh();
  app.snapshots[0].resolve({ busy: true, pending_perm: permissionPrompt("17") });
  await pending;
  assert.deepEqual(app.view().disabled, [true, true, true, true]);
  response.reject(new Error("network down"));
  await click;
  const error = app.controller.getState().error;
  const failed = app.refresh();
  app.snapshots[1].resolve({ busy: true, pending_perm: permissionPrompt("17", { hint: "Snapshot guidance." }) });
  await failed;
  assert.equal(app.controller.getState().error, error);
  assert.equal(app.view().hint, "Snapshot guidance. " + error);
  assert.deepEqual(app.view().disabled, [false, false, false, false]);
});

test("an authoritative replacement starts enabled and invalidates the old response", async () => {
  const response = deferred();
  const app = permissionAppHarness(() => response.promise);
  app.showPermission(permissionPrompt("17"));
  const click = app.click(3);
  const replacement = app.refresh();
  app.snapshots[0].resolve({ busy: true, pending_perm: permissionPrompt("18", { hint: "Snapshot replacement." }) });
  await replacement;
  assert.equal(app.view().id, "18");
  assert.deepEqual(app.view().disabled, [false, false, false, false]);
  const before = app.view();
  response.resolve({ status: 204 });
  await click;
  assert.deepEqual(app.view(), before);
});

for (const result of ["success", "failure"]) {
  test("authoritative absence invalidates the attempt before its old " + result, async () => {
    const old = deferred();
    const next = deferred();
    const app = permissionAppHarness(({ payload }) => payload.permission_id === "17" ? old.promise : next.promise);
    app.showPermission(permissionPrompt("17"));
    const oldClick = app.click(3);
    const cleared = app.refresh();
    app.snapshots[0].resolve({ busy: false });
    await cleared;
    assert.equal(app.view().visible, false);
    app.showPermission(permissionPrompt("18"));
    const nextClick = app.click(1);
    const before = app.view();
    if (result === "success") old.resolve({ status: 204 });
    else old.reject(new Error("old failure"));
    await oldClick;
    assert.deepEqual(app.view(), before);
    next.resolve({ status: 204 });
    await nextClick;
  });
}

test("done invalidates old snapshots, preserves a newer prompt, and lets finishTurnUI refresh reconcile", async () => {
  const app = permissionAppHarness();
  app.showPermission(permissionPrompt("18", { hint: "New turn guidance." }));
  const revision = app.controller.getRevision();
  const stale = app.refresh();
  assert.equal(app.dispatch({ type: "done" }), true);
  assert.equal(app.snapshots.length, 2, "finishTurnUI fetched state");
  assert.equal(app.controller.reconcile(null, revision), false);
  assert.equal(app.view().visible, true);
  assert.equal(app.view().hint, "New turn guidance.");
  app.snapshots[0].resolve({ busy: false });
  await stale;
  assert.equal(app.view().id, "18");
  app.snapshots[1].resolve({ busy: true, pending_perm: permissionPrompt("18", { hint: "Authoritative guidance." }) });
  await app.refreshes[1];
  assert.equal(app.view().id, "18");
  assert.equal(app.view().hint, "Authoritative guidance.");
  const cleared = app.refresh();
  app.snapshots[2].resolve({ busy: false });
  await cleared;
  assert.equal(app.view().visible, false);
});

test("idle restart resets acknowledged IDs and rejects the old process", async () => {
  const app = permissionAppHarness();
  app.showPermission(permissionPrompt("17", { permission_epoch: "old" }));
  await app.click(3);
  assert.equal(app.calls[0].payload.permission_epoch, "old");
  const idle = app.refresh();
  app.snapshots[0].resolve({ busy: false, permission_epoch: "new" });
  await idle;
  app.showPermission(permissionPrompt("1", { permission_epoch: "new" }));
  assert.equal(app.view().id, "1");
  app.showPermission(permissionPrompt("18", { permission_epoch: "old" }));
  assert.equal(app.view().id, "1");
  await app.click(3);
  assert.equal(app.calls[1].payload.permission_epoch, "new");
});

test("a restarted process invalidates an old pending response even with the same ID", async () => {
  const old = deferred();
  const app = permissionAppHarness(() => old.promise);
  app.showPermission(permissionPrompt("1", { permission_epoch: "old" }));
  const click = app.click(3);
  app.controller.setEpoch("new");
  app.showPermission(permissionPrompt("1", { permission_epoch: "new" }));
  old.resolve({ status: 204 });
  await click;
  assert.equal(app.view().visible, true);
  assert.equal(app.view().id, "1");
  assert.deepEqual(app.view().disabled, [false, false, false, false]);
});

for (const failure of ["http", "network"]) {
  test("late " + failure + " failure after done requests fresh reconciliation", async () => {
    const response = deferred();
    const app = permissionAppHarness(() => response.promise);
    app.showPermission(permissionPrompt("17"));
    const click = app.click(3);
    app.dispatch({ type: "done" });
    assert.equal(app.snapshots.length, 1);
    if (failure === "http") response.resolve({ status: 409 });
    else response.reject(new Error("response lost after cancel"));
    await click;
    assert.equal(app.snapshots.length, 2, "a fresh post-failure snapshot must be fetched");
    app.snapshots[0].resolve({ busy: false });
    await app.refreshes[0];
    app.snapshots[1].resolve({ busy: false });
    await app.refreshes[1];
    assert.equal(app.view().visible, false);
  });
}

test("clicking an enabled retained prompt after done still reconciles its rejection", async () => {
  const app = permissionAppHarness(() => ({ status: 409 }));
  app.showPermission(permissionPrompt("17"));
  app.dispatch({ type: "done" });
  await app.click(3);
  assert.equal(app.snapshots.length, 2);
  app.snapshots[0].resolve({ busy: false });
  await app.refreshes[0];
  app.snapshots[1].resolve({ busy: false });
  await app.refreshes[1];
  assert.equal(app.view().visible, false);
});

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
  return { calls, elements, panels, context };
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

test("summarizes bounded contradiction states without evidence text", () => {
  assert.equal(contradictionSummary({
    state: "CONFIRMED",
    signals: [{ positive_origin: "secret" }],
  }), "Contradictory evidence confirmed (1 signal); diagnose and recheck before continuing");
  assert.equal(contradictionSummary({
    state: "ADVISORY",
    signals: [{ positive_origin: "secret" }, { negative_origin: "instruction" }],
    signals_truncated: true,
  }), "Contradictory evidence is unverified (2 signals; some signals omitted); it cannot select an action");
  assert.equal(contradictionSummary({ state: "CONFIRMED", signals: [] }), "");
  assert.equal(contradictionSummary({ state: "NONE", signals: [{ positive_origin: "secret" }] }), "");
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

test("setup treats Claude CLI as optional", async () => {
  const harness = setupHarness({
    status: {
      components: [
        { id: "home", ok: true, can_fix: false, detail: "ready" },
        { id: "git", ok: true, can_fix: false, detail: "ready" },
        { id: "codex-cli", ok: true, can_fix: false, detail: "ready" },
        { id: "claude-cli", ok: false, can_fix: true, detail: "optional" },
      ],
      logged_in: false,
    },
  });
  await settleSetup();

  await harness.elements.get("install").onclick();
  const nextButton = harness.elements.get("next");
  assert.equal(nextButton.disabled, false);
  await nextButton.onclick();
  assert.equal(harness.panels[2].hidden, false);
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

test("malformed setup installation restores the explicit action", async () => {
  const harness = setupHarness(() => ({
    ok: true,
    json: async () => {
      throw new Error("invalid install response");
    },
  }));
  await settleSetup();

  const installButton = harness.elements.get("install");
  await installButton.onclick();

  assert.equal(installButton.disabled, false);
  assert.equal(installButton.textContent, "Install missing pieces");
  assert.equal(harness.elements.get("stage-err").textContent, "invalid install response");
});

// Deletion recovery executes the separate controller and the actual session UI.
const { createChatDeletionRecoveryController } = require("./contracts.js");

function deletionClock() {
  let time = 100000;
  let nextID = 0;
  const timers = new Map();
  return {
    now: () => time,
    setTimer(callback, delay) {
      const id = ++nextID;
      timers.set(id, { callback, at: time + delay });
      return id;
    },
    clearTimer: (id) => timers.delete(id),
    advance(ms) {
      time += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.at <= time) {
          timers.delete(id);
          timer.callback();
        }
      }
    },
    timerCount: () => timers.size,
  };
}

function deletedChat(id = "current", undoID = "undo-1", expires = 130000) {
  return { id, title: "Chat " + id, current_id: "rotated", was_current: id === "current",
    undo_id: undoID, undo_expires_at: new Date(expires).toISOString() };
}

function deletionRecord(data = deletedChat()) {
  return { ...data, expires_at: data.undo_expires_at, messages: ["must never be retained"] };
}

function sessionResponse(data, status = 200) {
  return { status, ok: status >= 200 && status < 300, json: async () => data };
}

function deletionControllerHarness(send) {
  const clock = deletionClock();
  let epoch = 0;
  const calls = [];
  const deletes = [];
  const restores = [];
  const errors = [];
  const renders = [];
  const controller = createChatDeletionRecoveryController({
    ...clock, getViewEpoch: () => epoch,
    send(payload) { calls.push(payload); return send(payload); },
    render: (state) => renders.push(state),
    onDelete: (data) => deletes.push(data), onRestore: (data) => restores.push(data),
    onError: (message) => errors.push(message),
  });
  return { controller, clock, calls, deletes, restores, errors, renders,
    moveView: () => { epoch++; },
    reconcile: (record, revision = controller.getRevision()) => controller.reconcile(record, revision, epoch) };
}

function deletionAppHarness(send = ({ payload }) => sessionResponse(
  payload.action === "delete" ? deletedChat(payload.id) : { id: "current", title: "Recovered", current_id: "rotated" }
)) {
  const script = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
  const start = script.indexOf("const chatDeletionRecovery = ");
  const end = script.indexOf("\nasync function refresh(", start);
  assert.ok(start >= 0 && end > start, "actual session wiring is present");
  const elements = new Map();
  const makeElement = (tag = "div") => {
    const classes = new Set();
    const el = {
      tagName: tag.toUpperCase(), children: [], dataset: {}, attributes: {}, hidden: true,
      disabled: false, textContent: "", value: "", className: "", parentNode: null,
      setAttribute(name, value) { this.attributes[name] = String(value); },
      appendChild(child) { child.parentNode = this; this.children.push(child); return child; },
      append(...children) { for (const child of children) this.appendChild(child); },
      replaceChildren(...children) { this.children = []; this.append(...children); },
      addEventListener(type, listener) { this[type] = listener; },
      classList: { toggle() {}, add: (name) => classes.add(name), remove: (name) => classes.delete(name) },
      focus() {},
    };
    Object.defineProperty(el, "innerHTML", {
      get: () => "", set() { el.replaceChildren(); },
    });
    return el;
  };
  const element = (id) => {
    if (!elements.has(id)) elements.set(id, makeElement());
    return elements.get(id);
  };
  const calls = [];
  const snapshots = [];
  const errors = [];
  const refreshes = [];
  const clock = deletionClock();
  const context = {
    window: { PicogentWebContracts: {
      createChatDeletionRecoveryController: (options) => createChatDeletionRecoveryController({ ...options, ...clock }),
    } },
    document: { createElement: makeElement }, $: element,
    threadList: element("thread-list"), threadSearch: element("thread-search"),
    recentRecoveryEl: element("recent-recovery"), recentSessionListEl: element("recent-session-list"),
    emptyEl: element("empty"), promptEl: element("prompt"),
    viewEpoch: 0, refreshGeneration: 0, sessionId: "current", busy: false, historyReplayPending: false,
    threadsCache: [{ id: "current", title: "Current" }, { id: "other", title: "Other" }],
    transcript: ["current transcript"],
    clearTaskProgress() {}, setUndoAvailable() {}, setChatsOpen() {}, loadHeroPrompts() {},
    clearLog() { context.transcript = []; },
    replayMessages(messages) { context.transcript = messages; },
    add(role, message) { assert.equal(role, "error"); errors.push(message); },
    refresh: async (history) => { refreshes.push(history); },
    fetch(url, options) {
      assert.equal(url, "/api/sessions", "no extra new-chat/cancel request is allowed");
      if (!options) {
        const request = deferred();
        snapshots.push(request);
        return request.promise;
      }
      const call = { url, options, payload: JSON.parse(options.body) };
      calls.push(call);
      return send(call);
    },
  };
  vm.runInNewContext(script.slice(start, end) + `
    globalThis.deletionApp = { controller: chatDeletionRecovery, loadThreads, deleteThread, undoDeleteThread,
      loadThread, newChat, renderThreads, renderRecentSessions };
  `, context, { filename: "app-session-wiring.js" });
  const app = context.deletionApp;
  app.renderThreads();
  app.renderRecentSessions();
  return {
    ...app, clock, calls, snapshots, errors, refreshes, elements,
    moveView() { context.viewEpoch++; },
    moveWorkspace() {
      context.viewEpoch++;
      context.sessionId = "workspace-current";
      context.threadsCache = [{ id: "workspace-current", title: "Workspace chat" }];
      context.transcript = ["workspace transcript"];
    },
    view() {
      return { id: context.sessionId, threads: JSON.parse(JSON.stringify(context.threadsCache)),
        transcript: [...context.transcript], bannerHidden: element("delete-recovery").hidden,
        bannerText: element("delete-recovery-text").textContent,
        undoDisabled: element("undo-delete").disabled, undoText: element("undo-delete").textContent,
        newDisabled: [element("new-chat").disabled, element("new-chat-top").disabled] };
    },
    async seed(record = deletionRecord()) {
      const request = app.loadThreads();
      snapshots.at(-1).resolve(sessionResponse({ sessions: context.threadsCache, current_id: context.sessionId, delete_undo: record }));
      await request;
    },
  };
}

test("deletion controller owns and deduplicates delete/restore before transport yields", async () => {
  const first = deferred();
  const second = deferred();
  const h = deletionControllerHarness((payload) => payload.action === "delete" ? first.promise : second.promise);
  const deletion = h.controller.delete("current");
  assert.equal(h.calls.length, 1);
  assert.equal(h.controller.getState().pending, true);
  assert.equal(await h.controller.delete("current"), false);
  assert.equal(await h.controller.delete("other"), false);
  first.resolve(sessionResponse(deletedChat()));
  assert.equal(await deletion, true);
  assert.deepEqual(h.calls[0], { action: "delete", id: "current" });
  const revision = h.controller.getRevision();
  const restore = h.controller.restore("undo-1");
  assert.equal(h.controller.reconcile(null, revision, 0), false);
  assert.equal(await h.controller.restore("undo-1"), false);
  assert.equal(await h.controller.delete("other"), false);
  second.resolve(sessionResponse({ id: "current", title: "Recovered", current_id: "rotated" }));
  assert.equal(await restore, true);
  assert.deepEqual(h.calls[1], { action: "undo_delete", undo_id: "undo-1" });
  assert.equal(h.deletes.length, 1);
  assert.equal(h.restores.length, 1);
  assert.equal(h.controller.getState().recovery, null);
  assert.equal(h.clock.timerCount(), 0);
});

test("session UI uses accessible sibling buttons, disables pending controls, and honors server rotation", async () => {
  const response = deferred();
  const app = deletionAppHarness(() => response.promise);
  const row = app.elements.get("thread-list").children[0];
  assert.equal(row.tagName, "DIV");
  assert.deepEqual(row.children.map((child) => child.tagName), ["BUTTON", "BUTTON"]);
  const [open, remove] = row.children;
  assert.equal(open.parentNode, remove.parentNode);
  assert.equal(open.attributes["aria-label"], "Open chat: Current");
  assert.equal(remove.attributes["aria-label"], "Delete chat: Current");
  const deleting = remove.onclick();
  assert.equal(app.calls.length, 1);
  const pendingRow = app.elements.get("thread-list").children[0];
  assert.deepEqual(pendingRow.children.map((child) => child.disabled), [true, true]);
  assert.equal(app.elements.get("recent-session-list").children[0].disabled, true);
  assert.deepEqual(app.view().newDisabled, [true, true]);
  await open.onclick(); // Even a saved old handler is guarded while deletion is pending.
  await remove.onclick();
  await app.newChat();
  assert.equal(app.calls.length, 1);
  response.resolve(sessionResponse(deletedChat()));
  assert.equal(await deleting, true);
  assert.equal(app.view().id, "rotated");
  assert.deepEqual(app.view().transcript, []);
  assert.deepEqual(app.view().threads.map((s) => s.id), ["other"]);
  assert.deepEqual(app.refreshes, [true]);
  assert.equal(app.calls.length, 1, "server already created the replacement");
  assert.equal(app.calls[0].options.method, "POST");
  assert.equal(app.calls[0].options.headers["content-type"], "application/json");
  assert.equal(app.view().bannerHidden, false);
  assert.match(app.view().bannerText, /Deleted.*Chat current/);
  assert.deepEqual(app.view().newDisabled, [false, false]);
});

for (const action of ["delete", "undo_delete"]) {
  for (const failure of [400, 401, 403, 404, 409, 410, 500, 503, "network", "synchronous", "json"]) {
    test("session " + action + " " + failure + " preserves chat/list/previous recovery and permits retry", async () => {
      let count = 0;
      const response = action === "delete" ? deletedChat() : { id: "other", title: "Recovered", current_id: "unrelated" };
      const app = deletionAppHarness(() => {
        if (count++ > 0) return sessionResponse(response);
        if (failure === "network") return Promise.reject(new Error("offline"));
        if (failure === "synchronous") throw new Error("transport unavailable");
        if (failure === "json") return { status: 200, json: async () => { throw new Error("broken JSON"); } };
        return sessionResponse(null, failure);
      });
      await app.seed(deletionRecord(deletedChat("other", "previous")));
      const previous = app.controller.getState().recovery;
      const before = app.view();
      const run = () => action === "delete" ? app.deleteThread("current") : app.elements.get("undo-delete").click();
      assert.equal(await run(), false);
      assert.equal(app.view().id, before.id);
      assert.deepEqual(app.view().threads, before.threads);
      assert.deepEqual(app.view().transcript, before.transcript);
      assert.deepEqual(app.controller.getState().recovery, previous);
      assert.equal(app.view().undoDisabled, false);
      assert.deepEqual(app.view().newDisabled, [false, false]);
      assert.equal(app.snapshots.length, 1, "failure must not reconcile away existing list/recovery");
      assert.match(app.errors.at(-1), /Please try again/);
      assert.equal(await run(), true);
      assert.equal(app.calls.length, 2);
      assert.deepEqual(app.calls[1].payload, app.calls[0].payload);
      if (action === "undo_delete") {
        assert.equal(app.view().id, "current", "undo response cannot force a chat switch");
        assert.deepEqual(app.view().transcript, before.transcript);
        assert.equal(app.view().bannerHidden, true);
      }
    });
  }
}

test("undo UI deduplicates restoration, retains transcript, and adds only history metadata", async () => {
  const response = deferred();
  const app = deletionAppHarness(() => response.promise);
  await app.seed(deletionRecord(deletedChat("deleted", "restore-me")));
  const before = app.view();
  const restoring = app.elements.get("undo-delete").click();
  assert.equal(app.view().undoDisabled, true);
  assert.equal(app.view().undoText, "Restoring…");
  assert.equal(app.elements.get("delete-recovery").attributes["aria-busy"], "true");
  assert.equal(await app.undoDeleteThread(), false);
  assert.equal(await app.deleteThread("current"), false);
  assert.equal(app.calls.length, 1);
  response.resolve(sessionResponse({ id: "deleted", title: "Recovered", current_id: "different" }));
  assert.equal(await restoring, true);
  assert.equal(app.view().id, before.id);
  assert.deepEqual(app.view().transcript, before.transcript);
  assert.deepEqual(app.view().threads[0], { id: "deleted", title: "Recovered" });
  assert.deepEqual(app.calls[0].payload, { action: "undo_delete", undo_id: "restore-me" });
  assert.equal(app.view().bannerHidden, true);
  assert.equal(app.refreshes.length, 0, "restoring history must not replay current-chat state");
});

test("reload exposes only live server recovery metadata and expires without another request", async () => {
  const app = deletionAppHarness();
  await app.seed(deletionRecord());
  assert.equal(app.calls.length, 0);
  assert.equal(app.view().bannerHidden, false);
  assert.deepEqual(Object.keys(app.controller.getState().recovery).sort(), ["expires_at", "id", "title", "undo_id", "was_current"]);
  assert.equal(app.clock.timerCount(), 1);
  app.clock.advance(29000);
  await app.seed(deletionRecord()); // A reload cannot renew the server's original deadline.
  assert.equal(app.clock.timerCount(), 1);
  app.clock.advance(1000);
  assert.equal(app.view().bannerHidden, true);
  assert.equal(app.view().undoDisabled, true);
  assert.equal(await app.undoDeleteThread(), false);
  assert.equal(app.calls.length, 0);
  assert.equal(app.clock.timerCount(), 0);
  const reloaded = deletionAppHarness();
  await reloaded.seed(deletionRecord(deletedChat("current", "expired", 99999)));
  assert.equal(reloaded.view().bannerHidden, true);
  await reloaded.seed(null);
  assert.equal(reloaded.controller.getState().recovery, null);
});

test("bounded recovery keeps one metadata record and never extends a deadline beyond 30 seconds", () => {
  const h = deletionControllerHarness(() => { throw new Error("no transport expected"); });
  for (let index = 0; index < 20; index++) {
    h.reconcile(deletionRecord(deletedChat("id-" + index, "token-" + index, 999999)));
    assert.equal(h.clock.timerCount(), 1);
  }
  assert.equal(h.controller.getState().recovery.undo_id, "token-19");
  assert.equal(h.controller.getState().recovery.expires_at, 130000);
  const copy = h.controller.getState().recovery;
  copy.undo_id = "tampered";
  assert.equal(h.controller.getState().recovery.undo_id, "token-19");
  h.clock.advance(30000);
  assert.equal(h.controller.getState().recovery, null);
});

test("GET snapshots begun before or during deletion cannot replace newer recovery or history", async () => {
  const response = deferred();
  const app = deletionAppHarness(() => response.promise);
  await app.seed(deletionRecord(deletedChat("old", "old-token")));
  const beforeDelete = app.loadThreads();
  const deleting = app.deleteThread("current");
  const duringDelete = app.loadThreads();
  response.resolve(sessionResponse(deletedChat("current", "new-token")));
  await deleting;
  const after = app.view();
  for (const index of [1, 2]) {
    app.snapshots[index].resolve(sessionResponse({ sessions: [{ id: "old-snapshot", title: "Old" }], current_id: "old-snapshot",
      delete_undo: index === 1 ? deletionRecord(deletedChat("old", "old-token")) : null }));
  }
  assert.equal(await beforeDelete, false);
  assert.equal(await duringDelete, false);
  assert.equal(app.controller.getState().recovery.undo_id, "new-token");
  assert.deepEqual(app.view(), after);
});

test("GET snapshots begun before/during undo cannot resurrect acknowledged recovery", async () => {
  const response = deferred();
  const app = deletionAppHarness(() => response.promise);
  const record = deletionRecord(deletedChat("deleted", "token"));
  await app.seed(record);
  const beforeUndo = app.loadThreads();
  const restoring = app.undoDeleteThread();
  const duringUndo = app.loadThreads();
  response.resolve(sessionResponse({ id: "deleted", title: "Recovered", current_id: "current" }));
  await restoring;
  const after = app.view();
  for (const index of [1, 2]) app.snapshots[index].resolve(sessionResponse({ sessions: [], current_id: "current", delete_undo: record }));
  assert.equal(await beforeUndo, false);
  assert.equal(await duringUndo, false);
  assert.deepEqual(app.view(), after);
  assert.equal(app.controller.getState().recovery, null);
});

test("same-generation concurrent GETs use the revision captured before fetching", async () => {
  const app = deletionAppHarness();
  const older = app.loadThreads();
  const newer = app.loadThreads();
  app.snapshots[1].resolve(sessionResponse({ sessions: [{ id: "newer", title: "Newer" }], current_id: "current",
    delete_undo: deletionRecord(deletedChat("newer", "new-token")) }));
  assert.equal(await newer, true);
  const after = app.view();
  app.snapshots[0].resolve(sessionResponse({ sessions: [], current_id: "current", delete_undo: null }));
  assert.equal(await older, false);
  assert.deepEqual(app.view(), after);
  assert.equal(app.controller.getState().recovery.undo_id, "new-token");
});

for (const result of ["success", "http", "network", "json body"]) {
  for (const action of ["delete", "undo_delete"]) {
    test("old workspace " + action + " " + result + " cannot replace new recovery, list, or transcript", async () => {
      const response = deferred();
      const body = deferred();
      const app = deletionAppHarness(() => response.promise);
      await app.seed(deletionRecord(deletedChat("deleted", "old-token")));
      const pending = action === "delete" ? app.deleteThread("current") : app.undoDeleteThread();
      if (result === "json body") {
        response.resolve({ status: 200, json: () => body.promise });
        await Promise.resolve();
        await Promise.resolve();
      }
      app.moveWorkspace();
      await app.seed(deletionRecord(deletedChat("workspace-deleted", "workspace-token")));
      const after = app.view();
      const revision = app.controller.getRevision();
      const data = action === "delete" ? deletedChat() : { id: "deleted", title: "Old", current_id: "old-current" };
      if (result === "network") response.reject(new Error("old offline"));
      else if (result === "json body") body.resolve(data);
      else response.resolve(sessionResponse(data, result === "http" ? 503 : 200));
      assert.equal(await pending, false);
      assert.deepEqual(app.view(), after);
      assert.equal(app.controller.getRevision(), revision);
      assert.equal(app.controller.getState().recovery.undo_id, "workspace-token");
      assert.equal(app.errors.length, 0);
      assert.equal(app.refreshes.length, 0);
    });
  }
}

test("workspace GET and view-epoch ABA snapshots cannot overwrite the current workspace", async () => {
  const app = deletionAppHarness();
  const old = app.loadThreads();
  app.moveWorkspace();
  await app.seed(deletionRecord(deletedChat("workspace-deleted", "workspace-token")));
  const after = app.view();
  app.snapshots[0].resolve(sessionResponse({ sessions: [{ id: "old", title: "Old workspace" }], current_id: "old", delete_undo: null }));
  assert.equal(await old, false);
  assert.deepEqual(app.view(), after);
  const h = deletionControllerHarness(() => sessionResponse(deletedChat()));
  const revision = h.controller.getRevision();
  h.moveView();
  h.moveView();
  assert.equal(h.controller.reconcile(deletionRecord(), revision, 0), false);
  assert.equal(h.controller.getState().recovery, null);
});

test("a late old controller attempt cannot settle a new pending request or its retry error", async () => {
  const old = deferred();
  const next = deferred();
  const h = deletionControllerHarness(({ id }) => id === "current" ? old.promise : next.promise);
  const oldDelete = h.controller.delete("current");
  h.moveView();
  h.reconcile(deletionRecord(deletedChat("previous", "previous-token")));
  const newDelete = h.controller.delete("other");
  const before = h.controller.getState();
  old.resolve(sessionResponse(deletedChat()));
  assert.equal(await oldDelete, false);
  assert.deepEqual(h.controller.getState(), before);
  next.resolve(sessionResponse(null, 503));
  assert.equal(await newDelete, false);
  assert.equal(h.controller.getState().recovery.undo_id, "previous-token");
  assert.match(h.controller.getState().error, /HTTP 503.*try again/);
});

for (const failure of ["http", "network", "json"]) {
  test("sessions GET " + failure + " preserves reload recovery/history and allows fresh retry", async () => {
    const app = deletionAppHarness();
    await app.seed();
    const before = app.view();
    const request = app.loadThreads();
    if (failure === "network") app.snapshots[1].reject(new Error("offline"));
    else if (failure === "json") app.snapshots[1].resolve({ status: 200, json: async () => { throw new Error("broken JSON"); } });
    else app.snapshots[1].resolve(sessionResponse(null, 503));
    assert.equal(await request, false);
    assert.deepEqual(app.view(), before);
    assert.match(app.errors.at(-1), /Couldn't load chats.*try again/);
    await app.seed();
    assert.deepEqual(app.view(), before);
  });
}
