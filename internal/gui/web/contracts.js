/* Small, executable contracts shared by the primary chat UI and its tests. */

(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.PicogentWebContracts = api;
})(typeof window !== "undefined" ? window : globalThis, function () {
  function createPrimaryEventDispatcher(handlers) {
    const h = handlers || {};
    return {
      dispatch(event) {
        if (!event || typeof event.type !== "string") return false;
        switch (event.type) {
          case "assistant_delta":
            if (typeof h.assistantDelta === "function") h.assistantDelta(event.text || "");
            return true;
          case "assistant_final":
            if (typeof h.assistantFinal === "function") h.assistantFinal(event.text || "");
            return true;
          case "done":
            if (typeof h.done === "function") h.done(event);
            return true;
          case "prompts_refresh": {
            const kind = event.text || "all";
            if ((kind === "main" || kind === "all") && typeof h.promptsRefresh === "function") {
              h.promptsRefresh(true, kind, event);
            }
            return true;
          }
          default:
            return false;
        }
      },
    };
  }

  function mainPromptRequest(force) {
    return {
      url: "/api/prompts?kind=main",
      options: {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ kind: "main", refresh: !!force }),
      },
    };
  }

  return Object.freeze({ createPrimaryEventDispatcher, mainPromptRequest });
});
