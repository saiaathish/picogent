const compsEl = document.getElementById("comps");
const loginsEl = document.getElementById("logins");
const logEl = document.getElementById("install-log");
const installBtn = document.getElementById("install");
const startBtn = document.getElementById("start");
const errEl = document.getElementById("finish-err");

function paint(st) {
  compsEl.innerHTML = "";
  (st.components || []).forEach((c) => {
    const li = document.createElement("li");
    li.className = c.ok ? "ok" : "miss";
    li.innerHTML = `<strong>${c.name}</strong><span>${c.ok ? "ready" : c.detail}</span>`;
    compsEl.appendChild(li);
  });
  loginsEl.innerHTML = "";
  (st.logins || []).forEach((l) => {
    const card = document.createElement("div");
    card.className = "login-card" + (l.ok ? " on" : "");
    const p = document.createElement("p");
    p.innerHTML = `<strong>Log in to ${l.name}</strong><br>${l.detail}`;
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
  if (st.model) document.getElementById("model").value = st.model;
  if (st.log) {
    logEl.hidden = false;
    logEl.textContent = st.log;
  }
  startBtn.disabled = !st.logged_in;
}

async function refresh() {
  const st = await (await fetch("/api/setup")).json();
  paint(st);
  return st;
}

async function install() {
  installBtn.disabled = true;
  installBtn.textContent = "Installing…";
  logEl.hidden = false;
  logEl.textContent = "running install commands…";
  const res = await fetch("/api/setup/install", { method: "POST" });
  const data = await res.json();
  logEl.textContent = data.log || data.error || "";
  if (data.status) paint(data.status);
  else await refresh();
  installBtn.disabled = false;
  installBtn.textContent = "Install missing pieces";
}

async function login(target) {
  const returnTo = location.origin + "/setup.html";
  const res = await fetch("/api/setup/login", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ target, return_to: returnTo }),
  });
  const text = await res.text();
  let data = {};
  try { data = JSON.parse(text); } catch { data = { error: text }; }
  if (!res.ok) {
    errEl.hidden = false;
    errEl.textContent = data.error || text;
    return;
  }
  if (data.url) {
    location.href = data.url;
    return;
  }
  errEl.hidden = false;
  errEl.textContent = data.hint || "Finish login, then this page will update.";
  const tick = setInterval(async () => {
    const st = await refresh();
    const hit = (st.logins || []).find((l) => l.id === target);
    if (hit && hit.ok) clearInterval(tick);
  }, 2000);
}

document.getElementById("start").onclick = async () => {
  errEl.hidden = true;
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
    errEl.hidden = false;
    errEl.textContent = await res.text();
    return;
  }
  location.href = "/";
};

installBtn.onclick = install;

const params = new URLSearchParams(location.search);
if (params.get("login") === "ok") {
  history.replaceState({}, "", "/setup.html");
}
if (params.get("error")) {
  errEl.hidden = false;
  errEl.textContent = params.get("error");
}

refresh().then((st) => {
  const missing = (st.components || []).some((c) => !c.ok && c.can_fix);
  if (missing) install();
});
