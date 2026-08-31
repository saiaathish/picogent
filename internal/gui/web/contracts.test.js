"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");

const { createPrimaryEventDispatcher, mainPromptRequest, completionProofSummary } = require("./contracts.js");

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
