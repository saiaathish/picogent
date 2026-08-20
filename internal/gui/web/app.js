/* Picogent chat UI — storage stays on server (~/.picogent/sessions), not localStorage */

const $ = (id) => document.getElementById(id);

const logEl = $("log");
const emptyEl = $("empty");
const thinkingEl = $("thinking");
const permEl = $("perm");
const permText = $("perm-text");
const promptEl = $("prompt");
const sendBtn = $("send");
const statusText = $("status-text");
const threadList = $("thread-list");
const shell = $("shell");
const reviewRail = $("rail-review");
const reviewPath = $("review-path");
const reviewScroll = $("review-scroll");
const reviewNote = $("review-note");
const reviewInput = $("review-input");
const modeSeg = $("mode-seg");
const scrim = $("scrim");
const threadSearch = $("thread-search");
let threadsCache = [];
let chatsOpen = false;

let ready = false;
let busy = false;
let sessionId = "";
let reviewOpen = false;
let reviewFile = "";
let highlight = { start: 0, end: 0 };

const REF_RE = /([`']?)((?:[\w.-]+\/)+[\w.-]+\.[a-zA-Z0-9]+)\1:(\d+)(?:-(\d+))?/g;
const LINE_RE = /(?:^|\s)(?:line|L)\s*(\d+)(?:\s*[-–]\s*(\d+))?/gi;

function syncEmpty() {
  emptyEl.hidden = logEl.children.length > 0;
}

function setThinking(on) {
  thinkingEl.classList.toggle("is-on", on);
  if (on) $("chat-scroll").scrollTop = $("chat-scroll").scrollHeight;
}

function syncScrim() {
  const on = chatsOpen || reviewOpen;
  scrim.hidden = !on;
}

function setChatsOpen(on) {
  chatsOpen = on;
  shell.classList.toggle("chats-open", on);
  syncScrim();
}

function setReviewOpen(on) {
  reviewOpen = on;
  reviewRail.hidden = !on;
  shell.classList.toggle("review-open", on);
  syncScrim();
}

function scrollChat() {
  const sc = $("chat-scroll");
  sc.scrollTop = sc.scrollHeight;
}

function addToolChip(label, done, path) {
  const chip = document.createElement("button");
  chip.type = "button";
  chip.className = "chip" + (done ? " is-done" : "");
  const mark = document.createElement("span");
  mark.textContent = done ? "✓" : "→";
  const body = document.createElement("span");
  body.className = "chip-body";
  body.textContent = label;
  chip.appendChild(mark);
  chip.appendChild(body);
  if (path) {
    chip.onclick = () => openReview(path);
  }
  logEl.appendChild(chip);
  syncEmpty();
  scrollChat();
  return chip;
}

function linkifyRefs(text, container) {
  container.textContent = "";
  let last = 0;
  const re = new RegExp(REF_RE.source, "g");
  let m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) {
      container.appendChild(document.createTextNode(text.slice(last, m.index)));
    }
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "code-ref";
    btn.textContent = m[2] + ":" + m[3] + (m[4] ? "-" + m[4] : "");
    const path = m[2];
    const start = +m[3];
    const end = m[4] ? +m[4] : start;
    btn.onclick = () => openReview(path, start, end, "Referenced in reply");
    container.appendChild(btn);
    last = m.index + m[0].length;
  }
  if (last < text.length) {
    container.appendChild(document.createTextNode(text.slice(last)));
  }
  if (last === 0) container.textContent = text;
}

function add(kind, text) {
  if (kind === "tool") {
    const isStart = text.startsWith("→ ");
    const name = isStart ? text.slice(2) : text;
    if (isStart) {
      addToolChip(name, false);
    } else {
      const last = logEl.querySelector(".chip:not(.is-done):last-of-type");
      if (last) {
        last.classList.add("is-done");
        last.querySelector("span").textContent = "✓";
        const body = last.querySelector(".chip-body");
        if (body && text && text.length < 80) body.textContent = text.replace(/\s+/g, " ").trim();
      } else {
        addToolChip(text, true);
      }
    }
    return;
  }

  const wrap = document.createElement("div");
  const role = kind === "you" || kind === "user" ? "user" : kind;
  wrap.className = "turn turn-" + role;

  if (role === "user") {
    const b = document.createElement("div");
    b.className = "bubble";
    b.textContent = text;
    wrap.appendChild(b);
  } else if (role === "assistant") {
    const c = document.createElement("div");
    c.className = "content";
    linkifyRefs(text, c);
    wrap.appendChild(c);
    applyRefsFromText(text);
  } else {
    wrap.textContent = text;
  }

  logEl.appendChild(wrap);
  syncEmpty();
  scrollChat();
}

function applyRefsFromText(text) {
  const re = new RegExp(REF_RE.source, "g");
  let m;
  while ((m = re.exec(text)) !== null) {
    openReview(m[2], +m[3], m[4] ? +m[4] : +m[3], "Assistant pointed here");
    break;
  }
}

function clearLog() {
  logEl.innerHTML = "";
  syncEmpty();
}

function replayMessages(msgs) {
  clearLog();
  for (const m of msgs) {
    if (m.role === "tool") addToolChip(m.text, true);
    else add(m.role === "user" ? "you" : m.role, m.text);
  }
}

async function loadThreads() {
  const data = await (await fetch("/api/sessions")).json();
  sessionId = data.current_id || sessionId;
  threadsCache = data.sessions || [];
  renderThreads();
}

function renderThreads() {
  const q = (threadSearch.value || "").toLowerCase().trim();
  threadList.innerHTML = "";
  for (const s of threadsCache) {
    if (q && !(s.title || "").toLowerCase().includes(q)) continue;
    const row = document.createElement("button");
    row.type = "button";
    row.className = "thread-item" + (s.id === sessionId ? " is-active" : "");
    row.dataset.id = s.id;
    const title = document.createElement("span");
    title.className = "thread-title";
    title.textContent = s.title || "New chat";
    const del = document.createElement("button");
    del.type = "button";
    del.className = "thread-del";
    del.textContent = "×";
    del.title = "Delete";
    del.onclick = (e) => {
      e.stopPropagation();
      deleteThread(s.id);
    };
    row.appendChild(title);
    row.appendChild(del);
    row.onclick = () => loadThread(s.id);
    threadList.appendChild(row);
  }
}

threadSearch.addEventListener("input", renderThreads);

async function loadThread(id) {
  if (busy) return;
  const data = await (
    await fetch("/api/sessions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "load", id }),
    })
  ).json();
  sessionId = data.id;
  replayMessages(data.messages || []);
  setChatsOpen(false);
  await loadThreads();
}

async function newChat() {
  if (busy) return;
  const data = await (
    await fetch("/api/sessions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "new" }),
    })
  ).json();
  sessionId = data.id;
  clearLog();
  await loadThreads();
  promptEl.focus();
}

async function deleteThread(id) {
  await fetch("/api/sessions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "delete", id }),
  });
  if (id === sessionId) await newChat();
  else await loadThreads();
}

async function refresh() {
  const s = await (await fetch("/api/state")).json();
  sessionId = s.session_id || sessionId;

  modeSeg.querySelectorAll("button").forEach((b) => {
    b.classList.toggle("is-on", b.dataset.mode === s.mode);
  });

  const bits = [s.model, s.workspace ? s.workspace.split("/").pop() : ""].filter(Boolean);
  if (s.mcp_tools) bits.push(s.mcp_tools + " MCP");
  statusText.textContent = bits.join(" · ") || "Ready";

  if (s.hint) statusText.textContent = s.hint;

  busy = !!s.busy;
  sendBtn.disabled = busy || !ready;
  setThinking(busy);
  await loadThreads();
}

modeSeg.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-mode]");
  if (!btn) return;
  await fetch("/api/mode", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ mode: btn.dataset.mode }),
  });
  refresh();
});

$("new-chat").onclick = newChat;

permEl.addEventListener("click", async (e) => {
  const t = e.target.closest("[data-allow], [data-turn]");
  if (!t) return;
  await fetch("/api/permission", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ allow: t.dataset.allow === "1", turn: t.dataset.turn === "1" }),
  });
  permEl.classList.remove("is-on");
});

$("composer").onsubmit = async (e) => {
  e.preventDefault();
  const prompt = promptEl.value.trim();
  if (!prompt || busy || !ready) return;
  add("you", prompt);
  promptEl.value = "";
  promptEl.style.height = "auto";
  busy = true;
  sendBtn.disabled = true;
  setThinking(true);
  const res = await fetch("/api/chat", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ prompt }),
  });
  if (res.status === 204) {
    busy = false;
    sendBtn.disabled = !ready;
    setThinking(false);
    return;
  }
  if (!res.ok) {
    add("error", await res.text());
    busy = false;
    sendBtn.disabled = !ready;
    setThinking(false);
  }
};

promptEl.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    $("composer").requestSubmit();
  }
});

promptEl.addEventListener("input", () => {
  promptEl.style.height = "auto";
  promptEl.style.height = Math.min(promptEl.scrollHeight, 160) + "px";
});

document.querySelectorAll(".rec").forEach((b) => {
  b.onclick = () => {
    promptEl.value = b.dataset.prompt || "";
    promptEl.focus();
    promptEl.dispatchEvent(new Event("input"));
  };
});

/* ─── Code review panel ─── */
async function openReview(path, start, end, note) {
  if (!path) return;
  setReviewOpen(true);
  reviewFile = path;
  highlight.start = start || 0;
  highlight.end = end || start || 0;
  reviewPath.textContent = path;
  if (note) {
    reviewNote.textContent = note;
    renderLineJumps();
  } else {
    reviewNote.textContent = "";
  }

  try {
    const data = await (await fetch("/api/file?path=" + encodeURIComponent(path))).json();
    renderReview(data);
    if (highlight.start) scrollToLine(highlight.start);
  } catch (err) {
    reviewScroll.innerHTML = '<p class="review-empty">' + err.message + "</p>";
  }
}

function renderLineJumps() {
  if (!highlight.start) return;
  reviewNote.innerHTML = "";
  reviewNote.appendChild(document.createTextNode("Highlight: "));
  const j = document.createElement("button");
  j.type = "button";
  j.className = "line-jump";
  j.textContent =
    highlight.start === highlight.end
      ? "L" + highlight.start
      : "L" + highlight.start + "–" + highlight.end;
  j.onclick = () => scrollToLine(highlight.start);
  reviewNote.appendChild(j);
}

function renderReview(data) {
  const table = document.createElement("table");
  table.className = "review-table";
  const tbody = document.createElement("tbody");
  for (const row of data.lines || []) {
    const tr = document.createElement("tr");
    if (
      highlight.start &&
      row.n >= highlight.start &&
      row.n <= (highlight.end || highlight.start)
    ) {
      tr.className = "is-hi";
    }
    if (row.n === highlight.start) tr.classList.add("is-cursor");
    const g = document.createElement("td");
    g.className = "gutter";
    g.textContent = row.n;
    const c = document.createElement("td");
    c.className = "code";
    c.textContent = row.t || " ";
    tr.appendChild(g);
    tr.appendChild(c);
    tbody.appendChild(tr);
  }
  table.appendChild(tbody);
  reviewScroll.innerHTML = "";
  reviewScroll.appendChild(table);
  if (data.truncated) {
    const p = document.createElement("p");
    p.className = "review-empty";
    p.textContent = "Showing first 800 of " + data.total + " lines.";
    reviewScroll.appendChild(p);
  }
}

function scrollToLine(n) {
  const rows = reviewScroll.querySelectorAll(".review-table tr");
  rows.forEach((tr) => {
    tr.classList.remove("is-cursor");
    if (+tr.querySelector(".gutter").textContent === n) {
      tr.classList.add("is-cursor");
      tr.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  });
}

$("review-ask").onsubmit = (e) => {
  e.preventDefault();
  const q = reviewInput.value.trim();
  if (!q) return;
  let prompt = q;
  const lineM = q.match(/(?:line|L)\s*(\d+)/i);
  if (lineM && reviewFile) {
    const ln = +lineM[1];
    openReview(reviewFile, ln, ln, "Jump to line " + ln);
    prompt = "Explain " + reviewFile + " line " + ln;
  } else if (reviewFile) {
    prompt = "Regarding " + reviewFile + ": " + q;
  }
  promptEl.value = prompt;
  reviewInput.value = "";
  promptEl.focus();
  $("composer").requestSubmit();
};

$("toggle-review").onclick = () => setReviewOpen(!reviewOpen);
$("close-review").onclick = () => setReviewOpen(false);
$("toggle-rail").onclick = () => setChatsOpen(!chatsOpen);
scrim.onclick = () => {
  setChatsOpen(false);
  setReviewOpen(false);
};
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    setChatsOpen(false);
    setReviewOpen(false);
  }
});

/* ─── SSE ─── */
const ev = new EventSource("/api/events");
ev.onmessage = (m) => {
  const e = JSON.parse(m.data);
  if (e.type === "hello") {
    ready = true;
    sendBtn.disabled = busy;
    return;
  }
  if (e.type === "permission") {
    permText.textContent = e.summary || "";
    permEl.classList.add("is-on");
    setThinking(false);
    return;
  }
  if (e.type === "review" && e.path) {
    openReview(e.path, e.line || 0, e.line_end || e.line || 0, "Reading file…");
    const last = logEl.querySelector(".chip:not(.is-done):last-of-type");
    if (last) {
      last.onclick = () => openReview(e.path);
      const body = last.querySelector(".chip-body");
      if (body) body.textContent = "read " + e.path;
    }
    return;
  }
  if (e.type === "done") {
    busy = false;
    sendBtn.disabled = !ready;
    permEl.classList.remove("is-on");
    setThinking(false);
    refresh();
    return;
  }
  if (e.type === "system") {
    add("system", e.text || "");
    return;
  }
  if (e.type === "tool") {
    add("tool", e.text || "");
    return;
  }
  add(e.type === "you" ? "you" : e.type, e.text || e.summary || e.type);
};
ev.onerror = () => {
  ready = false;
  statusText.textContent = "Reconnecting…";
};
ev.onopen = () => refresh();

syncEmpty();
refresh();
