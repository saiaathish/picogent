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
