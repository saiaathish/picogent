"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");

const { createPrimaryEventDispatcher, mainPromptRequest } = require("./contracts.js");

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
