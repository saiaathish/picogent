let state = {};
let modelOptionsCache = [];
let extPage = 1;
let extKind = "";
let extQuery = "";
let extLoading = false;

/* ─── Tab navigation ─── */
const navBtns = document.querySelectorAll(".settings-nav-btn");
const panelSettings = document.getElementById("panel-settings");
const panelExtensions = document.getElementById("panel-extensions");

function switchTab(tab) {
  navBtns.forEach((b) => {
    const on = b.dataset.tab === tab;
    b.classList.toggle("is-on", on);
    b.setAttribute("aria-selected", on ? "true" : "false");
  });
  panelSettings.hidden = tab !== "settings";
  panelExtensions.hidden = tab !== "extensions";
  if (tab === "extensions" && !document.getElementById("ext-list").children.length) {
    browseExtensions(true);
  }
  history.replaceState(null, "", tab === "extensions" ? "#extensions" : "#settings");
}

navBtns.forEach((b) => {
  b.onclick = () => switchTab(b.dataset.tab);
});

if (location.hash === "#extensions") switchTab("extensions");

/* ─── Settings form ─── */
function modelOptionsForProvider(s, provider) {
  if (provider === "quadcode") return s.model_options_quadcode || s.model_options || [];
  return s.model_options_codex || s.model_options || [];
}

function fillModelSelect(options, selected) {
  modelOptionsCache = options || [];
  const sel = document.getElementById("model");
  sel.innerHTML = "";
  for (const opt of modelOptionsCache) {
    const o = document.createElement("option");
    o.value = opt.value;
    o.textContent = opt.label;
    if (opt.value === selected) o.selected = true;
    sel.appendChild(o);
  }
  if (!sel.value && sel.options.length) sel.value = "auto";
  updateModelDesc();
}

function updateModelDesc() {
  const val = document.getElementById("model").value;
  const opt = modelOptionsCache.find((o) => o.value === val);
  document.getElementById("model-desc").textContent = opt?.description || "";
}

function syncProviderUI() {
  document.getElementById("anthropic-block").hidden =
    document.getElementById("provider").value !== "quadcode";
}

function renderLast(router) {
  const box = document.getElementById("router-last");
  const isAuto = document.getElementById("model").value === "auto";
  const last = router?.last;
  if (!isAuto || (!last?.model && !last?.label)) {
    box.hidden = true;
    return;
  }
  box.hidden = false;
  const label = last.label || last.model || last.tier;
  document.getElementById("router-last-main").textContent =
    label + (last.model ? ` · ${last.model}` : "");
  document.getElementById("router-last-reason").textContent = last.reason || "";
}

async function load() {
  const s = await (await fetch("/api/settings")).json();
  state = s;
  document.getElementById("workspace").value = s.workspace || "";
  document.getElementById("mode").value = s.mode || "safe";
  document.getElementById("provider").value = s.provider === "quadcode" ? "quadcode" : "codex";
  fillModelSelect(modelOptionsForProvider(s, document.getElementById("provider").value), s.model || "auto");
  syncProviderUI();
  renderLast(s.router);

  const conn = document.getElementById("connection");
  if (s.codex || s.has_anthropic_key || s.has_api_key) {
    conn.innerHTML = 'Connected. <a href="/setup.html">Setup</a>';
  } else {
    conn.innerHTML = 'Not connected. <a href="/setup.html">Finish setup</a>';
  }
}

document.getElementById("provider").onchange = () => {
  syncProviderUI();
  fillModelSelect(modelOptionsForProvider(state, document.getElementById("provider").value), "auto");
};

document.getElementById("model").onchange = () => {
  updateModelDesc();
  renderLast(state.router || {});
};

document.getElementById("workspace-pick").onclick = async () => {
  const status = document.getElementById("status");
  status.textContent = "Choose a folder…";
  const res = await fetch("/api/folder/pick", { method: "POST" });
  if (res.status === 204) {
    status.textContent = "";
    return;
  }
  if (!res.ok) {
    status.textContent = await res.text();
    return;
  }
  const data = await res.json();
  document.getElementById("workspace").value = data.path || "";
  status.textContent = "Selected — click Save.";
};

document.getElementById("form").onsubmit = async (e) => {
  e.preventDefault();
  const status = document.getElementById("status");
  const save = document.getElementById("save");
  status.textContent = "Saving…";
  save.disabled = true;
  const body = {
    workspace: document.getElementById("workspace").value,
    mode: document.getElementById("mode").value,
    provider: document.getElementById("provider").value,
    model: document.getElementById("model").value,
  };
  const key = document.getElementById("anthropic-key").value.trim();
  if (key) body.anthropic_api_key = key;
  const res = await fetch("/api/settings", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  save.disabled = false;
  status.textContent = res.ok ? "Saved." : "Couldn't save.";
  if (res.ok) {
    await load();
    setTimeout(() => (status.textContent = ""), 2000);
  }
};

load();

/* ─── Extensions ─── */
function kindLabel(kind) {
  if (kind === "mcp") return "MCP";
  if (kind === "skill") return "Skill";
  if (kind === "plugin") return "Plugin";
  return "Ext";
}

function reliabilityLabel(r) {
  if (r === "high") return "★★★";
  if (r === "good") return "★★";
  if (r === "fair") return "★";
  return "";
}

function renderExtRow(it) {
  const row = document.createElement("article");
  row.className = "ext-row" + (it.installed || it.active ? " is-installed" : "");
  row.dataset.id = it.id;

  const top = document.createElement("div");
  top.className = "ext-row-top";
  const badge = document.createElement("span");
  badge.className = "ext-kind-badge ext-kind-" + (it.kind || "mcp");
  badge.textContent = kindLabel(it.kind);
  const name = document.createElement("strong");
  name.textContent = it.name;
  top.appendChild(badge);
  top.appendChild(name);
  if (it.library === "claude-official") {
    const lib = document.createElement("span");
    lib.className = "ext-stars";
    lib.textContent = "Claude";
    top.appendChild(lib);
  } else if (it.stars) {
    const stars = document.createElement("span");
    stars.className = "ext-stars";
    stars.textContent = "★ " + it.stars.toLocaleString();
    top.appendChild(stars);
  }
  if (it.essential) {
    const tag = document.createElement("span");
    tag.className = "ext-rel";
    tag.textContent = "essential";
    top.appendChild(tag);
  } else if (it.active) {
    const tag = document.createElement("span");
    tag.className = "ext-rel";
    tag.textContent = "active";
    top.appendChild(tag);
  } else if (it.reliability) {
    const rel = document.createElement("span");
    rel.className = "ext-rel";
    rel.title = "Reliability";
    rel.textContent = reliabilityLabel(it.reliability);
    top.appendChild(rel);
  }

  const desc = document.createElement("p");
  desc.className = "ext-row-desc";
  desc.textContent = it.description || "No description.";

  const actions = document.createElement("div");
  actions.className = "ext-row-actions";

  if (it.source) {
    const link = document.createElement("a");
    link.href = it.source;
    link.target = "_blank";
    link.rel = "noopener";
    link.className = "ext-link";
    link.textContent = "GitHub";
    actions.appendChild(link);
  }

  if (it.installed || it.active) {
    const tag = document.createElement("span");
    tag.className = "ext-installed-tag";
    tag.textContent = it.essential ? "Essential" : "Active";
    actions.appendChild(tag);
  } else {
    const useBtn = document.createElement("button");
    useBtn.type = "button";
    useBtn.className = "ext-install-btn";
    useBtn.textContent = "Use now";
    useBtn.onclick = () => activateExt(it.id, useBtn, row);
    actions.appendChild(useBtn);
    if (it.library === "claude-official") {
      const keepBtn = document.createElement("button");
      keepBtn.type = "button";
      keepBtn.className = "ext-link";
      keepBtn.textContent = "Keep";
      keepBtn.onclick = () => keepExt(it.id, row);
      actions.appendChild(keepBtn);
    }
  }

  row.appendChild(top);
  row.appendChild(desc);
  row.appendChild(actions);
  return row;
}

async function activateExt(id, btn, row) {
  btn.disabled = true;
  btn.textContent = "…";
  const res = await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "activate", id }),
  });
  if (res.ok) {
    row.classList.add("is-installed");
    btn.replaceWith(Object.assign(document.createElement("span"), {
      className: "ext-installed-tag",
      textContent: "Active",
    }));
  } else {
    btn.disabled = false;
    btn.textContent = "Retry";
  }
}

async function keepExt(id, row) {
  await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "essential", id }),
  });
  row.classList.add("is-installed");
  browseExtensions(true);
}

async function installExt(id, btn, row) {
  btn.disabled = true;
  btn.textContent = "…";
  const res = await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "install", id, approve: true }),
  });
  if (res.ok) {
    row.classList.add("is-installed");
    btn.replaceWith(Object.assign(document.createElement("span"), {
      className: "ext-installed-tag",
      textContent: "Installed",
    }));
  } else {
    btn.disabled = false;
    btn.textContent = "Retry";
  }
}

function renderStats(stats) {
  const el = document.getElementById("ext-stats");
  if (!el) return;
  const bits = [];
  if (stats?.plugins) bits.push(stats.plugins.toLocaleString() + " plugins");
  if (stats?.mcp) bits.push(stats.mcp + " MCP");
  if (stats?.skills) bits.push(stats.skills + " skills");
  el.textContent = bits.length ? bits.join(" · ") : "";
}

async function browseExtensions(reset) {
  if (extLoading) return;
  extLoading = true;
  const list = document.getElementById("ext-list");
  if (reset) {
    extPage = 1;
    list.innerHTML = "";
  }
  const res = await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "browse", kind: extKind, query: extQuery, page: extPage }),
  });
  const data = await res.json();
  extLoading = false;
  renderStats(data.stats);
  for (const it of data.items || []) {
    list.appendChild(renderExtRow(it));
  }
  const more = document.getElementById("ext-more");
  more.hidden = !(data.items && data.items.length >= 20);
}

document.querySelectorAll(".ext-kind").forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll(".ext-kind").forEach((x) => x.classList.toggle("is-on", x === b));
    extKind = b.dataset.kind || "";
    browseExtensions(true);
  };
});

document.getElementById("ext-more").onclick = () => {
  extPage++;
  browseExtensions(false);
};

const assistantInput = document.getElementById("ext-assistant-input");
const assistantReply = document.getElementById("ext-assistant-reply");

async function runAssistant() {
  const query = assistantInput.value.trim();
  if (!query) return;
  assistantReply.hidden = false;
  assistantReply.textContent = "Searching…";
  const res = await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "assistant", query }),
  });
  const data = await res.json();
  assistantReply.textContent = data.message || "";
  const list = document.getElementById("ext-list");
  list.innerHTML = "";
  for (const it of data.items || []) {
    list.appendChild(renderExtRow(it));
  }
  document.getElementById("ext-more").hidden = true;
}

document.getElementById("ext-assistant-btn").onclick = runAssistant;
assistantInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    runAssistant();
  }
});
