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

  function completionProofSummary(proof) {
    if (!proof || typeof proof !== "object") return "";
    if (proof.ready === true) return "Completion proof ready";

    const reason = typeof proof.reason === "string" && proof.reason.trim()
      ? proof.reason.trim()
      : "completion proof is incomplete";
    const details = [];
    if (Array.isArray(proof.missing_criteria) && proof.missing_criteria.length) {
      details.push(proof.missing_criteria.length + " required " + (proof.missing_criteria.length === 1 ? "criterion" : "criteria") + " missing");
    }
    if (Array.isArray(proof.missing_requirements) && proof.missing_requirements.length) {
      details.push(proof.missing_requirements.length + " quality requirement" + (proof.missing_requirements.length === 1 ? "" : "s") + " missing");
    }
    if (proof.verification_required && proof.verification_current !== true) {
      details.push("workspace verification is not current");
    }
    return "Completion proof pending: " + reason + (details.length ? " (" + details.join(", ") + ")" : "");
  }

  return Object.freeze({ createPrimaryEventDispatcher, mainPromptRequest, completionProofSummary });
});
