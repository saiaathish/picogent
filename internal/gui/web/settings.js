let state = {};
let modelOptionsCache = [];

function modelOptionsForProvider(s, provider) {
  if (provider === "quadcode") {
    return s.model_options_quadcode || s.model_options || [];
  }
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
  const isClaude = document.getElementById("provider").value === "quadcode";
  document.getElementById("anthropic-block").hidden = !isClaude;
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
  document.getElementById("provider").value =
    s.provider === "quadcode" ? "quadcode" : "codex";
  const provider = document.getElementById("provider").value;
  fillModelSelect(modelOptionsForProvider(s, provider), s.model || "auto");
  syncProviderUI();
  renderLast(s.router);

  const conn = document.getElementById("connection");
  if (s.codex || s.has_anthropic_key || s.has_api_key) {
    conn.innerHTML =
      'Connected. Need to reconnect? <a href="/setup.html">Open setup</a>.';
  } else {
    conn.innerHTML =
      'Not connected yet. <a href="/setup.html">Finish setup</a> to log in.';
  }
}

document.getElementById("provider").onchange = () => {
  syncProviderUI();
  const provider = document.getElementById("provider").value;
  fillModelSelect(modelOptionsForProvider(state, provider), "auto");
};

document.getElementById("model").onchange = () => {
  updateModelDesc();
  renderLast(state.router || {});
};

document.getElementById("workspace-pick").onclick = async () => {
  const status = document.getElementById("status");
  status.textContent = "Choose a folder in Finder…";
  const res = await fetch("/api/projects", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "pick" }),
  });
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
  status.textContent = "Folder selected — click Save.";
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
  status.textContent = res.ok ? "Saved." : "Couldn't save — try again.";
  if (res.ok) {
    await load();
    setTimeout(() => (status.textContent = ""), 2000);
  }
};

load();
