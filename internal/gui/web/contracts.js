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

  function createPermissionResponseController({ send, render, onReconcile }) {
    const emptyState = () => ({ permission: null, pending: false, error: "", attempt: null });
    let state = emptyState();
    let revision = 0;
    let epoch = "";
    let reconcileAfterSettled = false;
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
    const setEpoch = (nextEpoch = "") => {
      if (nextEpoch === epoch) return;
      epoch = nextEpoch;
      acknowledgedID = "";
      reconcileAfterSettled = false;
      replace(null); // Invalidate responses from the previous process, even while idle.
    };

    return {
      getState,
      setEpoch,
      getRevision: () => revision,
      invalidateSnapshots() { revision++; reconcileAfterSettled = true; },
      show(permission) {
        if (permission?.permission_epoch && !epoch) setEpoch(permission.permission_epoch);
        if ((permission?.permission_epoch || "") !== epoch) return false;
        if (!permission?.permission_id || wasAcknowledged(permission.permission_id)) return false;
        replace(permission);
        return true;
      },
      reconcile(permission, expectedRevision, nextEpoch = "") {
        if (expectedRevision !== revision) return false;
        setEpoch(nextEpoch);
        reconcileAfterSettled = false;
        const next = permission?.permission_id ? permission : null;
        // Legacy fixtures have no process identity. Production epochs must
        // change before acknowledged IDs can become actionable again.
        if (!epoch && next && wasAcknowledged(next.permission_id)) acknowledgedID = "";
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
          }, epoch);
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
            if (reconcileAfterSettled && owner.error) {
              reconcileAfterSettled = false;
              if (typeof onReconcile === "function") onReconcile();
            }
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

  // One server-owned deletion record and one request; no transcript or ID history.
  function createChatDeletionRecoveryController({ send, render, onDelete, onRestore, onError,
    getViewEpoch = () => 0, now = Date.now, setTimer = setTimeout, clearTimer = clearTimeout }) {
    let epoch = getViewEpoch();
    let revision = 0;
    let recovery = null;
    let attempt = null;
    let error = "";
    let timer = null;
    const snapshot = () => ({ recovery: recovery ? { ...recovery } : null,
      pending: !!attempt, action: attempt?.action || "", error });
    const paint = () => render(snapshot());
    const stopTimer = () => {
      if (timer !== null) clearTimer(timer.handle);
      timer = null;
    };
    const syncView = () => {
      const next = getViewEpoch();
      if (next === epoch) return;
      epoch = next;
      recovery = null;
      attempt = null;
      error = "";
      revision++;
      stopTimer();
      paint();
    };
    const expire = () => {
      if (!recovery || recovery.expires_at > now()) return;
      recovery = null;
      error = "";
      revision++;
      stopTimer();
      paint();
    };
    const armExpiry = () => {
      stopTimer();
      if (!recovery) return;
      const scheduled = { handle: null };
      timer = scheduled;
      scheduled.handle = setTimer(() => {
        if (timer !== scheduled) return;
        timer = null;
        syncView();
        expire();
        armExpiry();
      }, Math.max(0, recovery.expires_at - now()));
    };
    const readRecovery = (record) => {
      if (!record || typeof record.id !== "string" || !record.id ||
          typeof record.undo_id !== "string" || !record.undo_id) return null;
      const expires = typeof record.expires_at === "number"
        ? record.expires_at : Date.parse(record.expires_at);
      if (!Number.isFinite(expires) || expires <= now()) return null;
      return { id: record.id, title: String(record.title || "New chat").slice(0, 160),
        undo_id: record.undo_id, expires_at: Math.min(expires, now() + 30000),
        was_current: record.was_current === true };
    };
    const owns = (owner) => attempt === owner && epoch === owner.epoch && getViewEpoch() === owner.epoch;

    async function request(action, id) {
      syncView();
      expire();
      if (attempt || !id || (action === "undo_delete" && recovery?.undo_id !== id)) return false;
      const owner = { epoch, action, id, recovery };
      attempt = owner;
      error = "";
      revision++;
      paint(); // Claim and disable synchronously, before transport can yield.
      const failure = action === "delete" ? "Couldn't delete this chat" : "Couldn't restore this chat";
      try {
        const response = await send(action === "delete" ? { action, id } : { action, undo_id: id });
        if (!owns(owner)) return false;
        if (response?.status !== 200) {
          throw new Error(failure + (response?.status ? " (HTTP " + response.status + ")" : "") + ". Please try again.");
        }
        const data = await response.json();
        if (!owns(owner)) return false;
        if (!data || typeof data.id !== "string" ||
            data.id !== (action === "delete" ? id : owner.recovery.id) ||
            typeof data.current_id !== "string" || !data.current_id) {
          throw new Error(failure + ". Please try again.");
        }
        if (action === "delete" && (typeof data.undo_id !== "string" || !data.undo_id ||
            !Number.isFinite(Date.parse(data.undo_expires_at)) || typeof data.was_current !== "boolean")) {
          throw new Error(failure + ". Please try again.");
        }
        recovery = action === "delete" ? readRecovery({ ...data, expires_at: data.undo_expires_at }) : null;
        attempt = null;
        revision++;
        armExpiry();
        paint();
        if (action === "delete") onDelete?.(data);
        else onRestore?.(data);
        return true;
      } catch (err) {
        if (!owns(owner)) return false;
        attempt = null;
        error = err?.message?.startsWith(failure) ? err.message : failure + ". Please try again.";
        revision++;
        paint();
        onError?.(error);
        return false;
      }
    }

    return {
      getState() { syncView(); expire(); return snapshot(); },
      getRevision() { syncView(); expire(); return revision; },
      reconcile(record, expectedRevision, expectedEpoch) {
        syncView();
        expire();
        if (expectedEpoch !== epoch || expectedRevision !== revision || attempt) return false;
        const next = readRecovery(record);
        if (next?.undo_id === recovery?.undo_id && next) {
          next.expires_at = Math.min(next.expires_at, recovery.expires_at);
        } else {
          error = "";
        }
        recovery = next;
        revision++;
        armExpiry();
        paint();
        return true;
      },
      delete: (id) => request("delete", id),
      restore: (undoID) => request("undo_delete", undoID),
    };
  }

  return Object.freeze({ createPrimaryEventDispatcher, mainPromptRequest, createPermissionResponseController, completionProofSummary, contradictionSummary, createChatDeletionRecoveryController });
});
