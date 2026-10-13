const STAGES = ["Agent", "Tools", "Log in", "Project"];
const TOTAL = STAGES.length;

let current = 0;
let status = {};
const providerInputs = [...document.querySelectorAll('input[name="provider"]')];
const knownProviders = ["codex", "quadcode", "opencode", "antigravity", "openai"];
let selectedProvider = sessionStorage.getItem("picogent-setup-provider");
if (!knownProviders.includes(selectedProvider)) selectedProvider = "codex";
let refreshNumber = 0;
let loginPoll;

const compsEl = document.getElementById("comps");
const loginsEl = document.getElementById("logins");
const logEl = document.getElementById("install-log");
const installBtn = document.getElementById("install");
const backBtn = document.getElementById("back");
const nextBtn = document.getElementById("next");
const errEl = document.getElementById("finish-err");
const stageErr = document.getElementById("stage-err");

function showError(msg) {
  const el = current === TOTAL - 1 ? errEl : stageErr;
  errEl.hidden = el !== errEl;
  stageErr.hidden = el !== stageErr;
  el.hidden = false;
  el.textContent = msg;
}

function clearError() {
  errEl.hidden = true;
  stageErr.hidden = true;
}
const dots = [...document.querySelectorAll(".step-dot")];
const panels = [...document.querySelectorAll(".setup-stage")];

function toolsReady(st) {
  return (st.components || []).length > 0 && st.components.every((c) => c.ok);
}

function canAdvance(st, stage) {
  if (stage === 0) return knownProviders.includes(selectedProvider);
  if (stage === 1) return toolsReady(st);
  if (stage === 2) return !!st.logged_in || (selectedProvider === "openai" && !!document.getElementById("openai-key").value.trim());
  return true;
}

function showStage(n) {
  current = Math.max(0, Math.min(TOTAL - 1, n));
  panels.forEach((p, i) => {
    p.hidden = i !== current;
  });
  dots.forEach((d, i) => {
    d.classList.toggle("is-active", i === current);
    d.classList.toggle("active", i === current);
    d.classList.toggle("is-done", i < current || (i === 1 && toolsReady(status)) || (i === 2 && canAdvance(status, 2)));
    d.classList.toggle("done", i < current || (i === 1 && toolsReady(status)) || (i === 2 && canAdvance(status, 2)));
  });
  backBtn.disabled = current === 0;
  const onLast = current === TOTAL - 1;
  nextBtn.textContent = onLast ? "Start chatting" : "Next";
  nextBtn.disabled = !canAdvance(status, current);
}

let modelOptionsCache = [];

function updateSetupModelDesc() {
  const val = document.getElementById("model").value;
  const opt = modelOptionsCache.find((o) => o.value === val);
  const el = document.getElementById("model-desc");
  if (el) el.textContent = opt?.description || "";
}

function paint(st) {
  status = st;
  providerInputs.forEach((input) => {
    input.checked = input.value === selectedProvider;
    input.disabled = !!st.busy;
  });
  compsEl.innerHTML = "";
  (st.components || []).forEach((c) => {
    const li = document.createElement("li");
    li.className = c.ok ? "is-done" : status.busy && !c.ok ? "is-running" : "";
    const icon = document.createElement("span");
    icon.className = "setup-check-icon";
    icon.textContent = c.ok ? "✓" : "·";
    const details = document.createElement("span");
    const name = document.createElement("strong");
    name.textContent = c.name;
    const hint = document.createElement("small");
    hint.textContent = c.ok ? "ready" : c.detail;
    details.append(name, document.createElement("br"), hint);
    li.append(icon, details);
    compsEl.appendChild(li);
  });

  loginsEl.innerHTML = "";
  document.getElementById("openai-key-block").hidden = selectedProvider !== "openai";
  (selectedProvider === "openai" ? [] : st.logins || []).forEach((l) => {
    const card = document.createElement("div");
    card.className = "login-card" + (l.ok ? " on" : "");
    const p = document.createElement("p");
    const name = document.createElement("strong");
    name.textContent = l.name;
    const detail = document.createElement("span");
    detail.className = "login-detail";
    detail.textContent = l.detail;
    p.append(name, document.createElement("br"), detail);
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = l.ok ? "Connected" : l.button;
    btn.disabled = !!l.ok;
    btn.onclick = () => login(l.id);
    card.appendChild(p);
    card.appendChild(btn);
    loginsEl.appendChild(card);
  });
  document.getElementById("login-intro").textContent = selectedProvider === "openai"
    ? (st.logged_in ? "An API key is already configured. Enter a new one only to replace it." : "Enter your OpenAI API key to continue.")
    : `Connect ${st.logins?.[0]?.name || "your agent"}, then return here.`;

  if (st.workspace) document.getElementById("workspace").value = st.workspace;
  if (st.mode) document.getElementById("mode").value = st.mode;
  const modeHint = document.getElementById("mode-override-hint");
  if (modeHint) {
    modeHint.hidden = !st.mode_overridden;
    if (st.mode_overridden) {
      modeHint.textContent = `PICOGENT_MODE keeps this setup session in ${String(st.active_mode || "").toUpperCase()} mode. Your saved choice applies next time.`;
    }
  }
  const modelSel = document.getElementById("model");
  const openaiModel = document.getElementById("openai-model");
  modelSel.hidden = selectedProvider === "openai";
  openaiModel.hidden = selectedProvider !== "openai";
  document.getElementById("model-label").htmlFor = selectedProvider === "openai" ? "openai-model" : "model";
  if (selectedProvider === "openai") {
    if (!openaiModel.value) openaiModel.value = st.model || "gpt-4.1-mini";
    document.getElementById("model-desc").textContent = "Enter a model ID supported by OpenAI Chat Completions.";
  }
  if (st.model_options && st.model_options.length) {
    modelOptionsCache = st.model_options;
    modelSel.innerHTML = "";
    for (const opt of modelOptionsCache) {
      const o = document.createElement("option");
      o.value = opt.value;
      o.textContent = opt.label;
      modelSel.appendChild(o);
    }
  }
  modelSel.value = st.model || modelSel.options[0]?.value || "";
  if (![...modelSel.options].some((o) => o.value === modelSel.value) && modelSel.options.length) {
    modelSel.value = modelSel.options[0].value;
  }
  if (selectedProvider !== "openai") updateSetupModelDesc();
  if (st.log && st.provider === selectedProvider) {
    logEl.hidden = false;
    logEl.textContent = st.log;
  }

  installBtn.disabled = st.busy || toolsReady(st) || !(st.components || []).some((c) => !c.ok && c.can_fix);
  installBtn.textContent = toolsReady(st) ? "All tools ready" : st.busy ? "Installing…" : "Install missing pieces";
  document.getElementById("recheck").disabled = !!st.busy;

  showStage(current);
}

async function refresh() {
  const requestNumber = ++refreshNumber;
  const st = await (await fetch(`/api/setup?provider=${encodeURIComponent(selectedProvider)}`)).json();
  if (requestNumber !== refreshNumber) return status;
  paint(st);
  return st;
}

async function install() {
  const provider = selectedProvider;
  installBtn.disabled = true;
  installBtn.textContent = "Installing…";
  logEl.hidden = false;
  logEl.textContent = "Running install commands…";
  const res = await fetch("/api/setup/install", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ provider }),
  });
  const data = await res.json();
  logEl.textContent = data.log || data.error || "";
  if (data.status && provider === selectedProvider) paint(data.status);
  else await refresh();
  if (!res.ok) showError(data.error || "Install failed. Check the log above.");
}

function canReach(st, n) {
  if (n <= current) return true;
  if (n === 1) return true;
  if (n === 2) return toolsReady(st);
  if (n === 3) return toolsReady(st) && canAdvance(st, 2);
  return false;
}

async function login(target) {
  clearError();
  if (loginPoll) clearInterval(loginPoll);
  const returnTo = location.origin + "/setup.html";
  const res = await fetch("/api/setup/login", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ target, provider: selectedProvider, return_to: returnTo }),
  });
  const text = await res.text();
  let data = {};
  try {
    data = JSON.parse(text);
  } catch {
    data = { error: text };
  }
  if (!res.ok) {
    showError(data.error || text);
    return;
  }
  if (data.url) {
    location.href = data.url;
    return;
  }
  showError(data.hint || "Finish login in the window that opened, then return here.");
  loginPoll = setInterval(async () => {
    const st = await refresh();
    const hit = (st.logins || []).find((l) => l.id === target);
    if (hit && hit.ok) {
      clearInterval(loginPoll);
      loginPoll = undefined;
      showStage(current);
    }
  }, 2000);
}

async function finish() {
  clearError();
  nextBtn.disabled = true;
  nextBtn.textContent = "Starting…";
  const res = await fetch("/api/setup/finish", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      workspace: document.getElementById("workspace").value,
      provider: selectedProvider,
      mode: document.getElementById("mode").value,
      model: selectedProvider === "openai" ? document.getElementById("openai-model").value.trim() : document.getElementById("model").value,
      api_key: selectedProvider === "openai" ? document.getElementById("openai-key").value.trim() : "",
    }),
  });
  if (!res.ok) {
    showError(await res.text());
    nextBtn.disabled = false;
    nextBtn.textContent = "Start chatting";
    return;
  }
  location.href = "/";
}

backBtn.onclick = () => showStage(current - 1);

nextBtn.onclick = async () => {
  if (current === TOTAL - 1) {
    await finish();
    return;
  }
  if (!canAdvance(status, current)) return;
  showStage(current + 1);
};

dots.forEach((dot) => {
  dot.onclick = () => {
    const target = Number(dot.dataset.step);
    if (canReach(status, target)) showStage(target);
  };
});

installBtn.onclick = install;
document.getElementById("recheck").onclick = refresh;
providerInputs.forEach((input) => {
  input.onchange = async () => {
    if (loginPoll) clearInterval(loginPoll);
    loginPoll = undefined;
    selectedProvider = input.value;
    sessionStorage.setItem("picogent-setup-provider", selectedProvider);
    clearError();
    logEl.hidden = true;
    await refresh();
  };
});
document.getElementById("model").onchange = updateSetupModelDesc;
document.getElementById("openai-key").oninput = () => showStage(current);

document.getElementById("workspace-pick").onclick = async () => {
  clearError();
  const btn = document.getElementById("workspace-pick");
  btn.disabled = true;
  try {
    const res = await fetch("/api/folder/pick", { method: "POST" });
    if (res.status === 204) return;
    if (!res.ok) {
      showError(await res.text());
      return;
    }
    const data = await res.json();
    document.getElementById("workspace").value = data.path || "";
  } catch (err) {
    showError(err.message || "Couldn't open folder picker");
  } finally {
    btn.disabled = false;
  }
};

const params = new URLSearchParams(location.search);
if (params.get("login") === "ok") {
  history.replaceState({}, "", "/setup.html");
}
if (params.get("error")) {
  showError(params.get("error"));
}

refresh().then((st) => {
  if (params.get("login") === "ok") {
    showStage(st.logged_in ? 3 : 2);
  } else {
    showStage(0);
  }
});
