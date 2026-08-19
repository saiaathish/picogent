const logEl = document.getElementById("log");
const permEl = document.getElementById("perm");
const permText = document.getElementById("perm-text");
const promptEl = document.getElementById("prompt");

function add(kind, text) {
  const li = document.createElement("li");
  li.className = kind;
  li.textContent = text;
  logEl.appendChild(li);
  logEl.scrollTop = logEl.scrollHeight;
}

async function refresh() {
  const s = await (await fetch("/api/state")).json();
  document.getElementById("mode-label").textContent = s.mode;
  document.getElementById("model").textContent = s.model;
  document.getElementById("ws").textContent = s.workspace;
  document.getElementById("provider").textContent = s.provider;
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
  if (!prompt) return;
  add("you", "you: " + prompt);
  promptEl.value = "";
  const res = await fetch("/api/chat", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
  if (!res.ok) add("error", await res.text());
};

const ev = new EventSource("/api/events");
ev.onmessage = (m) => {
  const e = JSON.parse(m.data);
  if (e.type === "permission") {
    permText.textContent = "Allow " + e.summary + "?";
    permEl.hidden = false;
    return;
  }
  add(e.type, e.text || e.summary || e.type);
};

refresh();
