/* Account quota is separate from this chat's context window. */
(() => {
  const byId = (id) => document.getElementById(id);
  const dialog = byId("usage-dialog");
  const open = byId("open-usage");
  const groupsEl = byId("usage-groups");
  const refreshButton = byId("refresh-usage");
  const providers = {
    codex: { label: "Codex", summary: "codex", note: "Codex limits are shared across your account, including other apps and chats.", url: "https://chatgpt.com/codex/settings/usage", link: "Open Codex usage dashboard ↗" },
    quadcode: { label: "Claude Code", summary: "claude", note: "Claude Code subscription limits are shared with your Claude account.", url: "https://claude.ai/settings/usage", link: "Open Claude usage settings ↗" },
    antigravity: { label: "Antigravity", note: "Antigravity reports quota per model. These allowances are shared across your account.", url: "https://antigravity.google/docs/cli/commands/usage/", link: "Open Antigravity usage guide ↗" },
  };
  let provider = "";
  let enabled = false;
  let selectedModel = "";
  let generation = 0;
  let pending = null;
  let snapshot = null;

  function windowName(window) {
    const seconds = window.window_seconds;
    if (!Number.isFinite(seconds) || seconds <= 0) return window.name || "Limit window";
    if (seconds % 604800 === 0) return `${seconds / 604800}-week window`;
    if (seconds % 86400 === 0) return `${seconds / 86400}-day window`;
    if (seconds % 3600 === 0) return `${seconds / 3600}-hour window`;
    if (seconds % 60 === 0) return `${seconds / 60}-minute window`;
    return `${seconds}-second window`;
  }

  function resetText(timestamp) {
    if (!Number.isFinite(timestamp) || timestamp <= 0) return "Reset time unavailable";
    const date = new Date(timestamp * 1000);
    if (!Number.isFinite(date.getTime())) return "Reset time unavailable";
    const minutes = Math.ceil((date.getTime() - Date.now()) / 60000);
    if (minutes <= 0) return "Reset due — refresh to check";
    const days = Math.floor(minutes / 1440);
    const hours = Math.floor((minutes % 1440) / 60);
    const left = days ? `${days}d ${hours}h` : hours ? `${hours}h ${minutes % 60}m` : `${minutes}m`;
    return `Resets in ${left} · ${date.toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}`;
  }

  function render(data) {
    snapshot = data;
    groupsEl.replaceChildren();
    byId("usage-summary").textContent = "";
    byId("usage-plan").textContent = "";
    byId("usage-updated").textContent = "";
    open.setAttribute("aria-label", "View usage limits");
    const details = providers[provider];
    byId("usage-note").hidden = !details;
    byId("usage-note").textContent = details?.note || "";
    const dashboard = dialog.querySelector(".usage-dashboard");
    dashboard.hidden = !details;
    if (details) { dashboard.href = details.url; dashboard.textContent = details.link; }
    const usage = data.usage;
    if (!data.available || !usage) {
      byId("usage-message").hidden = false;
      byId("usage-message").textContent = data.message || "Usage limits are unavailable right now.";
      open.title = "View usage limits";
      return;
    }
    const plan = String(usage.plan || "").replaceAll("_", " ");
    const label = details?.label || "Account";
    byId("usage-plan").textContent = plan ? `${label} · ${plan.charAt(0).toUpperCase()}${plan.slice(1)}` : label;
    const summaryID = data.summary_id || details?.summary || selectedModel;
    const mainGroup = (usage.groups || []).find(g => g.id === summaryID);
    const mainWindows = mainGroup?.windows || [];
    const mainRemaining = mainWindows.map(w => w.remaining_percent).filter(Number.isFinite);
    if (mainRemaining.length) {
      const lowest = Math.round(Math.min(...mainRemaining));
      byId("usage-summary").textContent = ` · ${lowest}% left`;
      open.title = `${mainGroup?.name || label} allowance: ${lowest}% remaining in the lowest reported window`;
      open.setAttribute("aria-label", `Usage limits. ${open.title}`);
    }
    let windowCount = 0;
    for (const group of usage.groups || []) {
      if (!group.windows?.length && group.allowed !== false && !group.limit_reached) continue;
      const section = document.createElement("section");
      section.className = "usage-group";
      const heading = document.createElement("h3");
      heading.textContent = group.name || "Allowance";
      section.append(heading);
      if (group.limit_reached || group.allowed === false) {
        const note = document.createElement("p");
        note.className = "usage-limit-note";
        note.textContent = group.limit_reached ? "Limit reached" : "This allowance is currently unavailable.";
        section.append(note);
      }
      for (const window of group.windows || []) {
        windowCount++;
        const name = windowName(window);
        const row = document.createElement("div");
        row.className = "usage-window";
        const head = document.createElement("div");
        head.className = "usage-window-head";
        const label = document.createElement("span");
        label.textContent = name;
        const remaining = document.createElement("strong");
        const percent = window.remaining_percent;
        remaining.textContent = Number.isFinite(percent) ? `${Math.round(percent)}% remaining` : "Unavailable";
        head.append(label, remaining);
        row.append(head);
        if (Number.isFinite(percent)) {
          const meter = document.createElement("progress");
          meter.max = 100;
          meter.value = Math.max(0, Math.min(100, percent));
          meter.setAttribute("aria-label", `${group.name}: ${name}, ${Math.round(percent)}% remaining`);
          meter.dataset.level = percent <= 10 ? "critical" : percent <= 25 ? "warning" : "ok";
          row.append(meter);
        }
        const reset = document.createElement("p");
        reset.className = "usage-reset";
        reset.textContent = resetText(window.resets_at);
        row.append(reset);
        section.append(row);
      }
      groupsEl.append(section);
    }
    byId("usage-message").hidden = windowCount > 0;
    byId("usage-message").textContent = "No limit windows were reported for this account.";
    const updated = new Date(usage.updated_at);
    if (Number.isFinite(updated.getTime())) {
      byId("usage-updated").textContent = `Updated ${updated.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}`;
    }
  }

  async function refresh() {
    if (!enabled || pending) return;
    const epoch = generation;
    const controller = new AbortController();
    pending = controller;
    refreshButton.disabled = true;
    try {
      const res = await fetch("/api/usage", { signal: controller.signal, cache: "no-store" });
      if (!res.ok) throw new Error("unavailable");
      const data = await res.json();
      if (epoch !== generation || data.provider !== provider) return;
      if (data.supported === false) { enabled = false; open.hidden = true; if (dialog.open) dialog.close(); }
      render(data);
    } catch (error) {
      if (epoch === generation && error.name !== "AbortError") render({ available: false, message: "Couldn't load usage limits. Try refreshing." });
    } finally {
      if (epoch === generation) {
        pending = null;
        refreshButton.disabled = false;
      }
    }
  }

  open.addEventListener("click", () => {
    if (!enabled) return;
    dialog.showModal();
    if (snapshot) render(snapshot); // Refresh the reset countdown when reopening.
    refresh();
  });
  byId("close-usage").addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", () => { if (!open.hidden) open.focus(); });
  dialog.addEventListener("click", (event) => { if (event.target === dialog) {
    const box = dialog.getBoundingClientRect();
    if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) dialog.close();
  } });
  refreshButton.addEventListener("click", refresh);
  window.PicogentUsage = {
    setProvider(next, supported = false, model = "") {
      const nextEnabled = supported && !!providers[next];
      if (provider === next && enabled === nextEnabled && selectedModel === model) return;
      provider = next;
      enabled = nextEnabled;
      selectedModel = model;
      generation++;
      pending?.abort();
      pending = null;
      open.hidden = !enabled;
      if (!enabled && dialog.open) dialog.close();
      render({ available: false, message: "Loading usage limits…" });
      refresh();
    },
  };
  setInterval(() => { if (!document.hidden && enabled) refresh(); }, 5 * 60 * 1000);
})();
