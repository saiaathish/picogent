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

  function createPermissionResponseController({ send, render }) {
    const emptyState = () => ({ permission: null, pending: false, error: "", attempt: null });
    let state = emptyState();
    let revision = 0;
    // Server IDs are increasing decimal uint64s. One high-water mark blocks
    // old events without an ID history; fresh authority can reset it on restart.
    let acknowledgedID = "";
    const wasAcknowledged = (id) => {
      if (!acknowledgedID) return false;
      if (id === acknowledgedID) return true;
      if (/^[1-9]\d*$/.test(id) && /^[1-9]\d*$/.test(acknowledgedID)) {
        return BigInt(id) <= BigInt(acknowledgedID);
      }
      return false;
    };
    const getState = () => ({ permission: state.permission, pending: state.pending, error: state.error });
    const paint = () => render(getState());
    const replace = (permission) => {
      if (permission && state.permission?.permission_id === permission.permission_id) {
        state.permission = permission; // Same-ID redraws retain the owned attempt/error.
      } else {
        state = { ...emptyState(), permission };
      }
      revision++;
      paint();
    };

    return {
      getState,
      getRevision: () => revision,
      invalidateSnapshots() { revision++; },
      show(permission) {
        if (!permission?.permission_id || wasAcknowledged(permission.permission_id)) return false;
        replace(permission);
        return true;
      },
      reconcile(permission, expectedRevision) {
        if (expectedRevision !== revision) return false;
        const next = permission?.permission_id ? permission : null;
        if (next && wasAcknowledged(next.permission_id)) acknowledgedID = "";
        replace(next);
        return true;
      },
      async respond(permissionID, choice) {
        if (!permissionID || state.permission?.permission_id !== permissionID || state.pending) return false;
        const owner = state;
        const attempt = {};
        const ownsAttempt = () => state === owner && state.attempt === attempt;
        owner.attempt = attempt;
        owner.pending = true;
        owner.error = "";
        revision++;
        paint(); // Disable every choice before sending or yielding to another click.
        try {
          const response = await send(permissionID, {
            allow: choice.allow === true,
            turn: choice.turn === true,
            always: choice.always === true,
          });
          if (!ownsAttempt()) return false;
          if (response?.status !== 204) {
            owner.error = "Choice wasn't acknowledged" +
              (response?.status ? " (HTTP " + response.status + ")" : "") + ". Please try again.";
            return false;
          }
          acknowledgedID = permissionID;
          replace(null);
          return true;
        } catch (_) {
          if (ownsAttempt()) owner.error = "Couldn't send this choice. Please try again.";
          return false;
        } finally {
          if (ownsAttempt()) {
            owner.pending = false;
            owner.attempt = null;
            revision++;
            paint();
          }
        }
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

  function contradictionSummary(report) {
    if (!report || typeof report !== "object") return "";
    const state = report.state;
    if (state !== "CONFIRMED" && state !== "ADVISORY") return "";
    const count = Array.isArray(report.signals) ? report.signals.length : 0;
    if (count < 1) return "";
    const label = count === 1 ? "signal" : "signals";
    const truncated = report.signals_truncated === true ? "; some signals omitted" : "";
    if (state === "CONFIRMED") {
      return "Contradictory evidence confirmed (" + count + " " + label + truncated + "); diagnose and recheck before continuing";
    }
    return "Contradictory evidence is unverified (" + count + " " + label + truncated + "); it cannot select an action";
  }

  return Object.freeze({ createPrimaryEventDispatcher, mainPromptRequest, createPermissionResponseController, completionProofSummary, contradictionSummary });
});
