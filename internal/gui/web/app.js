const logEl = document.getElementById("log");
const permEl = document.getElementById("perm");
const permText = document.getElementById("perm-text");
const promptEl = document.getElementById("prompt");
const sendBtn = document.getElementById("send");
const badge = document.getElementById("badge");
const hintEl = document.getElementById("hint");
let ready = false;
let busy = false;

function add(kind, text) {
  const li = document.createElement("li");
  li.className = kind;
  const who = document.createElement("span");
  who.className = "who";
  who.textContent = kind === "you" ? "You" : kind === "assistant" ? "Picogent" : kind === "tool" ? "Tool" : kind;
  const body = document.createElement("div");
  body.textContent = text;
  if (kind !== "tool") li.appendChild(who);
  li.appendChild(body);
  logEl.appendChild(li);
  logEl.scrollTop = logEl.scrollHeight;
  logEl.parentElement.scrollTop = logEl.parentElement.scrollHeight;
}

async function refresh() {
  const s = await (await fetch("/api/state")).json();
  document.getElementById("toggle-mode").textContent = s.mode === "fast" ? "Fast" : "Safe";
  document.getElementById("model").textContent = s.model || "";
  document.getElementById("workspace").textContent = s.workspace || "";
  if (s.codex) {
    badge.textContent = "Codex connected";
    badge.className = "badge on";
  } else if (s.provider === "ollama") {
    badge.textContent = "Ollama";
    badge.className = "badge on";
  } else if (s.hint) {
    badge.textContent = "Not connected";
    badge.className = "badge off";
  } else {
    badge.textContent = s.provider;
    badge.className = "badge on";
  }
  if (s.hint) {
    hintEl.hidden = false;
    hintEl.textContent = s.hint;
  } else {
    hintEl.hidden = true;
    hintEl.textContent = "";
  }
  busy = !!s.busy;
  sendBtn.disabled = busy || !ready;
}

document.getElementById("toggle-mode").onclick = async () => {
  const s = await (await fetch("/api/state")).json();
  const next = s.mode === "safe" ? "fast" : "safe";
  await fetch("/api/mode", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ mode: next }),
  });
  refresh();
};

document.getElementById("reset").onclick = async () => {
  await fetch("/api/reset", { method: "POST" });
  logEl.innerHTML = "";
  add("system", "New chat.");
};

permEl.addEventListener("click", async (e) => {
  const t = e.target;
  if (!t.dataset.allow && !t.dataset.turn) return;
  await fetch("/api/permission", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      allow: t.dataset.allow === "1",
      turn: t.dataset.turn === "1",
    }),
  });
  permEl.hidden = true;
});

document.getElementById("composer").onsubmit = async (e) => {
  e.preventDefault();
  const prompt = promptEl.value.trim();
  if (!prompt || busy || !ready) return;
  add("you", prompt);
  promptEl.value = "";
  busy = true;
  sendBtn.disabled = true;
  const res = await fetch("/api/chat", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
  if (!res.ok) {
    add("error", await res.text());
    busy = false;
    sendBtn.disabled = !ready;
  }
};

promptEl.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    document.getElementById("composer").requestSubmit();
  }
});

const ev = new EventSource("/api/events");
ev.onmessage = (m) => {
  const e = JSON.parse(m.data);
  if (e.type === "hello") {
    ready = true;
    sendBtn.disabled = busy;
    return;
  }
  if (e.type === "permission") {
    permText.textContent = "Allow " + e.summary + "?";
    permEl.hidden = false;
    return;
  }
  if (e.type === "done") {
    busy = false;
    sendBtn.disabled = !ready;
    permEl.hidden = true;
    refresh();
    return;
  }
  add(e.type, e.text || e.summary || e.type);
};
ev.onerror = () => {
  ready = false;
  badge.textContent = "reconnecting";
  badge.className = "badge off";
};
ev.onopen = () => refresh();

refresh();
