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
const reasoningEl = $("reasoning");
const overviewCard = $("overview-card");
const overviewPct = $("overview-pct");
const overviewBar = $("overview-bar");
const overviewList = $("overview-list");
const projectList = $("project-list");
const changesList = $("changes-list");
const changesSummary = $("changes-summary");
let threadsCache = [];
let chatsOpen = false;
let turnChanges = [];
let turnStats = { reads: 0, searches: 0, edits: 0, added: 0, removed: 0 };

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
  if (on) {
    resetReasoning();
    reasoningEl.hidden = false;
  }
  if (on) $("chat-scroll").scrollTop = $("chat-scroll").scrollHeight;
}

function resetReasoning() {
  turnChanges = [];
  turnStats = { reads: 0, searches: 0, edits: 0, added: 0, removed: 0 };
  reasoningEl.innerHTML = "";
  renderChangesPanel();
}

function addReasonStep(text, meta) {
  const step = document.createElement("div");
  step.className = "reason-step";
  const p = document.createElement("p");
  p.className = "reason-text";
  p.textContent = text;
  step.appendChild(p);
  if (meta) {
    const ul = document.createElement("ul");
    ul.className = "reason-stats";
    const li = document.createElement("li");
    li.textContent = meta;
    ul.appendChild(li);
    step.appendChild(ul);
  }
  reasoningEl.appendChild(step);
  scrollChat();
}

function updateReasonStats() {
  const parts = [];
  if (turnStats.reads) parts.push("Explored " + turnStats.reads + " file" + (turnStats.reads === 1 ? "" : "s"));
  if (turnStats.searches) parts.push(turnStats.searches + " search" + (turnStats.searches === 1 ? "" : "es"));
  if (turnStats.edits) {
    parts.push("Edited " + turnStats.edits + " file" + (turnStats.edits === 1 ? "" : "s"));
  }
  let summary = reasoningEl.querySelector(".reason-summary");
  if (!summary && (turnStats.edits || parts.length)) {
    summary = document.createElement("button");
    summary.type = "button";
    summary.className = "reason-summary";
    summary.onclick = () => {
      setReviewOpen(true);
      showReviewTab("changes");
    };
    reasoningEl.appendChild(summary);
  }
  if (summary) {
    let html = parts.join(" · ");
    if (turnStats.edits) {
      html = (html ? html + " · " : "") + "Edited " + turnStats.edits + " files";
      if (turnStats.added || turnStats.removed) {
        html += ' <span class="diff-add">+' + turnStats.added + '</span> <span class="diff-del">−' + turnStats.removed + "</span>";
      }
    }
    summary.innerHTML = html || "View changes";
    summary.hidden = !turnStats.edits && !parts.length;
  }
}

function renderOverview(ov) {
  if (!ov) {
    overviewCard.hidden = true;
    return;
  }
  overviewCard.hidden = false;
  const pct = ov.knowledge || 0;
  overviewPct.textContent = pct + "%";
  overviewBar.style.width = pct + "%";
  overviewList.innerHTML = "";
  for (const line of ov.overview || []) {
    const li = document.createElement("li");
    li.textContent = line;
    overviewList.appendChild(li);
  }
}

function renderChangesPanel() {
  changesList.innerHTML = "";
  if (!turnChanges.length) {
    changesList.innerHTML = '<p class="review-empty">File edits appear here with line counts. Click a file to review.</p>';
    changesSummary.textContent = "";
    return;
  }
  changesSummary.innerHTML =
    turnStats.edits +
    " files · " +
    '<span class="diff-add">+' +
    turnStats.added +
    '</span> <span class="diff-del">−' +
    turnStats.removed +
    "</span>";
  for (const c of turnChanges) {
    const row = document.createElement("button");
    row.type = "button";
    row.className = "change-row";
    const path = document.createElement("span");
    path.className = "change-path";
    path.textContent = c.path;
    const stats = document.createElement("span");
    stats.className = "change-stats";
    stats.innerHTML =
      '<span class="diff-add">+' + (c.added || 0) + '</span> <span class="diff-del">−' + (c.removed || 0) + "</span>";
    row.appendChild(path);
    row.appendChild(stats);
    row.onclick = () => openReview(c.path, 0, 0, "Changed this turn");
    changesList.appendChild(row);
  }
}

function showReviewTab(tab) {
  document.querySelectorAll(".review-tab").forEach((b) => {
    b.classList.toggle("is-on", b.dataset.tab === tab);
  });
  $("panel-review").hidden = tab !== "review";
  $("panel-changes").hidden = tab !== "changes";
}

document.querySelectorAll(".review-tab").forEach((b) => {
  b.onclick = () => showReviewTab(b.dataset.tab);
});

async function loadProjects() {
  const data = await (await fetch("/api/projects")).json();
  projectList.innerHTML = "";
  const active = (data.projects || []).find((p) => p.id === data.current_id);
  const sub = $("active-project");
  if (sub) {
    sub.textContent = active ? active.name + " · " + (active.path.split("/").slice(-2).join("/") || "") : "";
  }
  for (const p of data.projects || []) {
    const row = document.createElement("button");
    row.type = "button";
    row.className = "project-item" + (p.id === data.current_id ? " is-active" : "");
    const name = document.createElement("span");
    name.className = "project-name";
    name.textContent = p.name;
    const path = document.createElement("small");
    path.textContent = p.path.split("/").slice(-2).join("/");
    row.appendChild(name);
    row.appendChild(path);
    row.onclick = () => switchProject(p.id);
    projectList.appendChild(row);
  }
}

async function applyProjectSwitch(data) {
  sessionId = data.session_id || sessionId;
  if (data.messages) replayMessages(data.messages);
  else clearLog();
  setChatsOpen(true);
  await refresh();
}

async function pickProjectFolder() {
  if (busy) return;
  statusText.textContent = "Choose a folder in Finder…";
  const res = await fetch("/api/projects", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "pick" }),
  });
  if (res.status === 204) {
    await refresh();
    return;
  }
  if (!res.ok) {
    statusText.textContent = await res.text();
    return;
  }
  await applyProjectSwitch(await res.json());
}

async function switchProject(id) {
  if (busy) return;
  const res = await fetch("/api/projects", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "switch", id }),
  });
  if (!res.ok) return;
  await applyProjectSwitch(await res.json());
}

$("add-project").onclick = pickProjectFolder;

async function runTests() {
  if (busy) return;
  addReasonStep("Running tests…");
  await fetch("/api/test", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" });
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

  const bits = [];
  if (s.router?.enabled || s.model === "auto") {
    const last = s.router?.last;
    bits.push(last?.label ? "auto · " + last.label : "auto");
  } else if (s.model) {
    bits.push(s.model);
  }
  if (s.workspace) bits.push(s.workspace.split("/").pop());
  if (s.mcp_tools) bits.push(s.mcp_tools + " MCP");
  statusText.textContent = bits.join(" · ") || "Ready";

  if (s.hint) statusText.textContent = s.hint;

  renderOverview(s.overview);
  busy = !!s.busy;
  sendBtn.disabled = busy || !ready;
  setThinking(busy);
  await loadThreads();
  await loadProjects();
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
    if (b.dataset.action === "test") {
      runTests();
      return;
    }
    if (b.dataset.action === "pick") {
      pickProjectFolder();
      return;
    }
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
    const thinkLabel = reasoningEl.querySelector(".reason-thinking");
    if (thinkLabel) thinkLabel.remove();
    refresh();
    return;
  }
  if (e.type === "think") {
    if (e.status === "start") addReasonStep(e.text);
    if (e.status === "done") {
      const t = reasoningEl.querySelector(".reason-thinking");
      if (t) t.remove();
    }
    return;
  }
  if (e.type === "activity") {
    if (e.kind === "reset") return;
    if (e.kind === "read") turnStats.reads = e.count || turnStats.reads + 1;
    if (e.kind === "search") turnStats.searches = e.count || turnStats.searches + 1;
    if (e.kind === "edit") {
      turnStats.edits = e.count || turnStats.edits;
      turnStats.added = e.added || turnStats.added;
      turnStats.removed = e.removed || turnStats.removed;
    }
    updateReasonStats();
    return;
  }
  if (e.type === "change" && e.path) {
    turnChanges.push({ path: e.path, added: e.added || 0, removed: e.removed || 0 });
    turnStats.edits = turnChanges.length;
    turnStats.added = turnChanges.reduce((a, c) => a + (c.added || 0), 0);
    turnStats.removed = turnChanges.reduce((a, c) => a + (c.removed || 0), 0);
    updateReasonStats();
    renderChangesPanel();
    return;
  }
  if (e.type === "changes_summary") {
    turnStats.edits = e.count || turnStats.edits;
    turnStats.added = e.added || turnStats.added;
    turnStats.removed = e.removed || turnStats.removed;
    updateReasonStats();
    renderChangesPanel();
    return;
  }
  if (e.type === "test") {
    const note = document.createElement("div");
    note.className = "test-result" + (e.status === "fail" ? " is-fail" : " is-pass");
    note.innerHTML = "<strong>" + (e.text || "Tests") + "</strong>";
    if (e.summary) {
      const pre = document.createElement("pre");
      pre.textContent = e.summary.slice(0, 1200);
      note.appendChild(pre);
    }
    logEl.appendChild(note);
    syncEmpty();
    scrollChat();
    if (e.status === "fail") {
      promptEl.value = "Fix the failing tests";
      promptEl.focus();
    }
    return;
  }
  if (e.type === "overview") {
    fetch("/api/overview")
      .then((r) => r.json())
      .then(renderOverview);
    return;
  }
  if (e.type === "route") {
    const note = document.createElement("div");
    note.className = "route-chip";
    note.textContent = "Routed to " + (e.text || "model");
    if (e.summary) note.title = e.summary;
    logEl.appendChild(note);
    syncEmpty();
    scrollChat();
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
  if (busy && !reasoningEl.querySelector(".reason-thinking")) {
    const t = document.createElement("div");
    t.className = "reason-thinking";
    t.textContent = "Thinking";
    reasoningEl.appendChild(t);
    scrollChat();
  }
};
ev.onerror = () => {
  ready = false;
  statusText.textContent = "Reconnecting…";
};
ev.onopen = () => refresh();

syncEmpty();
refresh();
