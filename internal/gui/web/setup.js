const STAGES = ["Welcome", "Core tools", "Log in", "Your agent"];
const TOTAL = STAGES.length;

let current = 0;
let status = {};

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
  const required = (st.components || []).filter(
    (c) => c.id === "home" || c.id === "git" || c.id === "codex-cli" || c.id === "claude-cli"
  );
  return required.every((c) => c.ok);
}

function firstIncomplete(st) {
  if (!toolsReady(st)) return 1;
  if (!st.logged_in) return 2;
  return 0;
}

function canAdvance(st, stage) {
  if (stage === 0) return true;
  if (stage === 1) return toolsReady(st);
  if (stage === 2) return !!st.logged_in;
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
    d.classList.toggle("is-done", i < current || (i === 1 && toolsReady(status)) || (i === 2 && status.logged_in));
    d.classList.toggle("done", i < current || (i === 1 && toolsReady(status)) || (i === 2 && status.logged_in));
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
  compsEl.innerHTML = "";
  (st.components || []).forEach((c) => {
    const li = document.createElement("li");
    li.className = c.ok ? "is-done" : status.busy && !c.ok ? "is-running" : "";
    li.innerHTML = `<span class="setup-check-icon">${c.ok ? "✓" : "·"}</span><span><strong>${c.name}</strong><br><small>${c.ok ? "ready" : c.detail}</small></span>`;
    compsEl.appendChild(li);
  });

  loginsEl.innerHTML = "";
  (st.logins || []).forEach((l) => {
    const card = document.createElement("div");
    card.className = "login-card" + (l.ok ? " on" : "");
    const p = document.createElement("p");
    p.innerHTML = `<strong>${l.name}</strong><br><span class="login-detail">${l.detail}</span>`;
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = l.ok ? "Connected" : l.button;
    btn.disabled = !!l.ok;
    btn.onclick = () => login(l.id);
    card.appendChild(p);
    card.appendChild(btn);
    loginsEl.appendChild(card);
  });

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
  updateSetupModelDesc();
  if (st.log) {
    logEl.hidden = false;
    logEl.textContent = st.log;
  }

  installBtn.disabled = st.busy || toolsReady(st);
  installBtn.textContent = toolsReady(st) ? "All tools ready" : st.busy ? "Installing…" : "Install missing pieces";

  showStage(current);
}

async function refresh() {
  const st = await (await fetch("/api/setup")).json();
  paint(st);
  return st;
}

let installStarted = false;

async function install() {
  installStarted = true;
  installBtn.disabled = true;
  installBtn.textContent = "Installing…";
  logEl.hidden = false;
  logEl.textContent = "Running install commands…";
  const res = await fetch("/api/setup/install", { method: "POST" });
  const data = await res.json();
  logEl.textContent = data.log || data.error || "";
  if (data.status) paint(data.status);
  else await refresh();
  installStarted = false;
}

function canReach(st, n) {
  if (n <= current) return true;
  if (n === 1) return true;
  if (n === 2) return toolsReady(st);
  if (n === 3) return toolsReady(st) && st.logged_in;
  return false;
}

async function login(target) {
  clearError();
  const returnTo = location.origin + "/setup.html";
  const res = await fetch("/api/setup/login", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ target, return_to: returnTo }),
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
  const tick = setInterval(async () => {
    const st = await refresh();
    const hit = (st.logins || []).find((l) => l.id === target);
    if (hit && hit.ok) {
      clearInterval(tick);
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
      mode: document.getElementById("mode").value,
      model: document.getElementById("model").value,
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
document.getElementById("model").onchange = updateSetupModelDesc;

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
    showStage(firstIncomplete(st));
  }
});
