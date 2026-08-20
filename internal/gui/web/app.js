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
const contextBar = $("context-bar");
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
const projectList = $("project-list");
const changesList = $("changes-list");
const changesSummary = $("changes-summary");
const extRecsEl = $("ext-recs");
const extToastsEl = $("ext-toasts");
const permTitle = $("perm-title");
let threadsCache = [];
let chatsOpen = false;
let turnChanges = [];
let turnStats = { reads: 0, searches: 0, edits: 0, added: 0, removed: 0 };
let activityItems = [];
let activityPanel = null;

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
  thinkingEl.classList.toggle("is-on", false);
  if (on) {
    resetReasoning();
    reasoningEl.hidden = false;
  } else {
    reasoningEl.hidden = reasoningEl.children.length === 0;
  }
  if (on) $("chat-scroll").scrollTop = $("chat-scroll").scrollHeight;
}

function resetReasoning() {
  turnChanges = [];
  turnStats = { reads: 0, searches: 0, edits: 0, added: 0, removed: 0 };
  activityItems = [];
  activityPanel = null;
  reasoningEl.innerHTML = "";
  renderChangesPanel();
}

function ensureActivityPanel() {
  if (activityPanel) return activityPanel;
  const block = document.createElement("div");
  block.className = "activity-block";

  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "activity-toggle";
  toggle.innerHTML =
    '<svg class="activity-chevron" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>' +
    '<span class="activity-label">Working…</span>';

  const details = document.createElement("div");
  details.className = "activity-details";
  details.hidden = true;

  toggle.onclick = () => {
    const open = details.hidden;
    details.hidden = !open;
    toggle.classList.toggle("is-open", open);
  };

  block.appendChild(toggle);
  block.appendChild(details);
  reasoningEl.appendChild(block);
  activityPanel = { block, toggle, details, label: toggle.querySelector(".activity-label") };
  return activityPanel;
}

function pushActivity(kind, text, path) {
  const panel = ensureActivityPanel();
  if (path && !activityItems.some((a) => a.path === path && a.kind === kind)) {
    activityItems.push({ kind, text, path });
  } else if (!path && text) {
    activityItems.push({ kind, text, path: "" });
  }
  updateActivityPanel();
}

function updateActivityPanel() {
  const panel = ensureActivityPanel();
  const parts = [];
  if (turnStats.reads) parts.push("Explored " + turnStats.reads + " file" + (turnStats.reads === 1 ? "" : "s"));
  if (turnStats.searches) parts.push(turnStats.searches + " search" + (turnStats.searches === 1 ? "" : "es"));
  if (turnStats.edits) parts.push("Edited " + turnStats.edits + " file" + (turnStats.edits === 1 ? "" : "s"));
  panel.label.textContent = parts.length ? parts.join(" · ") : "Working…";

  panel.details.innerHTML = "";
  if (!activityItems.length) return;
  const ul = document.createElement("ul");
  ul.className = "activity-list";
  for (const item of activityItems.slice(-24)) {
    const li = document.createElement("li");
    if (item.path) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "activity-file";
      btn.textContent = item.text || item.path;
      btn.onclick = () => openReview(item.path);
      li.appendChild(btn);
    } else {
      li.textContent = item.text;
    }
    ul.appendChild(li);
  }
  panel.details.appendChild(ul);
}

function addReasonStep(text) {
  /* Plan steps fold into the activity summary — no verbose step list */
  if (text && text.length < 140) {
    pushActivity("plan", text);
  }
}

function updateReasonStats() {
  updateActivityPanel();
  let summary = reasoningEl.querySelector(".reason-summary");
  if (!summary && turnStats.edits) {
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
    let html = "";
    if (turnStats.edits) {
      html = "Edited " + turnStats.edits + " files";
      if (turnStats.added || turnStats.removed) {
        html += ' <span class="diff-add">+' + turnStats.added + '</span> <span class="diff-del">−' + turnStats.removed + "</span>";
      }
    }
    summary.innerHTML = html || "View changes";
    summary.hidden = !turnStats.edits;
  }
}

function renderContext(ctx) {
  if (!contextBar || !ctx) return;
  const pct = Math.min(100, Math.round((ctx.pct || 0) * 100));
  contextBar.hidden = false;
  contextBar.dataset.level = ctx.level || "ok";
  contextBar.title = (ctx.tokens || 0).toLocaleString() + " / " + (ctx.budget || 0).toLocaleString() + " est. tokens";
  const fill = contextBar.querySelector(".context-fill");
  const label = contextBar.querySelector(".context-label");
  if (fill) fill.style.width = pct + "%";
  if (label) {
    if (ctx.level === "critical") label.textContent = "Context " + pct + "% · compacting";
    else if (ctx.level === "warning") label.textContent = "Context " + pct + "%";
    else label.textContent = pct > 0 ? "Context " + pct + "%" : "";
    contextBar.hidden = pct <= 0 && ctx.level === "ok";
  }
}

function renderOverview(ov) {
  if (!ov || !ov.knowledge) {
    overviewCard.hidden = true;
    return;
  }
  overviewCard.hidden = false;
  const pct = ov.knowledge || 0;
  overviewPct.textContent = pct + "% explored";
  overviewBar.style.width = pct + "%";
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
  const prev = statusText.textContent;
  statusText.textContent = "Choose a folder…";
  try {
    const res = await fetch("/api/projects", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "pick" }),
    });
    if (res.status === 204) {
      statusText.textContent = prev || "Ready";
      return;
    }
    if (!res.ok) {
      statusText.textContent = await res.text();
      return;
    }
    await applyProjectSwitch(await res.json());
  } catch (err) {
    statusText.textContent = err.message || "Couldn't open folder picker";
  }
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
  pushActivity("test", "Running tests…");
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

function linkifyRefs(text, container) {
  if (window.renderContent) {
    window.renderContent(text, container, (path, start, end) => {
      openReview(path, start, end, "Referenced in reply");
    });
    return;
  }
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
    c.className = "content md";
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
    if (m.role === "tool") continue;
    add(m.role === "user" ? "you" : m.role, m.text);
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
  currentMode = s.mode || "safe";

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
  renderContext(s.context);
  if (logEl.children.length === 0 && s.messages?.length) {
    replayMessages(s.messages);
  }
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
  const t = e.target.closest("[data-allow], [data-turn], [data-always]");
  if (!t) return;
  await fetch("/api/permission", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      allow: t.dataset.allow === "1",
      turn: t.dataset.turn === "1",
      always: t.dataset.always === "1",
    }),
  });
  permEl.classList.remove("is-on");
});

/* ─── Extensions finder ─── */
function kindLabel(kind) {
  if (kind === "mcp") return "MCP";
  if (kind === "skill") return "Skill";
  if (kind === "plugin") return "Plugin";
  return kind || "Extension";
}

function renderExtRecommendations(items) {
  if (!extRecsEl || !items.length) return;
  extRecsEl.hidden = false;
  extRecsEl.innerHTML = "";
  const head = document.createElement("p");
  head.className = "ext-recs-head";
  head.textContent = "Suggested for this task";
  extRecsEl.appendChild(head);
  for (const it of items) {
    extRecsEl.appendChild(buildExtCard(it, { recommend: true }));
  }
}

function buildExtCard(it, opts) {
  const card = document.createElement("div");
  card.className = "ext-card";
  card.dataset.id = it.path || it.id;

  const badge = document.createElement("span");
  badge.className = "ext-badge";
  badge.textContent = kindLabel(it.kind);

  const title = document.createElement("strong");
  title.textContent = it.text || it.name;

  const desc = document.createElement("p");
  desc.className = "ext-desc";
  desc.textContent = it.summary || it.description || "";

  const row = document.createElement("div");
  row.className = "ext-actions";

  const installBtn = document.createElement("button");
  installBtn.type = "button";
  installBtn.className = "ext-install";
  installBtn.textContent = "Install";
  installBtn.onclick = () => installExtension(it.path || it.id, !!opts?.recommend);

  const dismissBtn = document.createElement("button");
  dismissBtn.type = "button";
  dismissBtn.className = "ext-dismiss ghost-btn";
  dismissBtn.textContent = "Not now";
  dismissBtn.onclick = () => dismissExtension(it.path || it.id, card);

  row.appendChild(installBtn);
  row.appendChild(dismissBtn);
  card.appendChild(badge);
  card.appendChild(title);
  card.appendChild(desc);
  if (it.status === "auth") {
    const hint = document.createElement("small");
    hint.className = "ext-auth-hint";
    hint.textContent = "Requires authorization after install";
    card.appendChild(hint);
  }
  card.appendChild(row);
  return card;
}

async function installExtension(id, fromRecommend) {
  const res = await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "install", id, approve: true }),
  });
  const data = await res.json().catch(() => ({}));
  if (data.needs_approval) {
    showExtApproval(data.item);
    return;
  }
  if (!res.ok) {
    add("error", data.message || "Install failed");
    return;
  }
  if (extRecsEl) {
    const card = extRecsEl.querySelector('[data-id="' + id + '"]');
    if (card) card.remove();
    if (!extRecsEl.querySelector(".ext-card")) extRecsEl.hidden = true;
  }
  if (data.auto) {
    showExtToast(data.result?.message || "Extension installed", data.undo_id, id);
  } else {
    add("system", data.result?.message || "Extension installed");
  }
  refresh();
}

function showExtApproval(item) {
  if (!item) return;
  renderExtRecommendations([{ path: item.id, text: item.name, summary: item.description, kind: item.kind, status: item.auth_required ? "auth" : "" }]);
}

async function dismissExtension(id, cardEl) {
  await fetch("/api/extensions", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ action: "dismiss", id }),
  });
  if (cardEl) cardEl.remove();
  if (extRecsEl && !extRecsEl.querySelector(".ext-card")) extRecsEl.hidden = true;
}

function showExtToast(message, undoId, extId) {
  if (!extToastsEl) return;
  const toast = document.createElement("div");
  toast.className = "ext-toast";
  const msg = document.createElement("span");
  msg.textContent = message;
  const undo = document.createElement("button");
  undo.type = "button";
  undo.className = "ext-undo";
  undo.textContent = "Undo";
  undo.onclick = async () => {
    await fetch("/api/extensions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "undo", undo_id: undoId }),
    });
    toast.remove();
    refresh();
  };
  toast.appendChild(msg);
  toast.appendChild(undo);
  extToastsEl.appendChild(toast);
  setTimeout(() => toast.classList.add("is-visible"), 10);
  setTimeout(() => {
    if (toast.parentNode) toast.remove();
  }, 30000);
}

function showAuthPrompt(name, hint, id) {
  if (!extToastsEl) return;
  const toast = document.createElement("div");
  toast.className = "ext-toast ext-auth";
  toast.innerHTML = "<strong>Authorize " + escapeHtml(name) + "</strong><p>" + escapeHtml(hint || "Add credentials in Settings → Extensions or ~/.picogent/mcp.yaml") + "</p>";
  const ok = document.createElement("button");
  ok.type = "button";
  ok.className = "ext-install";
  ok.textContent = "Done";
  ok.onclick = async () => {
    await fetch("/api/extensions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "auth_done", id }),
    });
    toast.remove();
  };
  toast.appendChild(ok);
  extToastsEl.appendChild(toast);
  setTimeout(() => toast.classList.add("is-visible"), 10);
}

function escapeHtml(s) {
  return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

let currentMode = "safe";

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

function finishTurnUI() {
  busy = false;
  sendBtn.disabled = !ready;
  setThinking(false);
  const thinkLabel = reasoningEl.querySelector(".reason-thinking");
  if (thinkLabel) thinkLabel.remove();
  if (activityPanel) {
    activityPanel.details.hidden = true;
    activityPanel.toggle.classList.remove("is-open");
  }
  reasoningEl.hidden = reasoningEl.children.length === 0;
  refresh();
}

/* ─── SSE ─── */
let ev;
function connectEvents() {
  if (ev) ev.close();
  ev = new EventSource("/api/events");
  ev.onopen = () => {
    refresh().catch(() => {});
  };
  ev.onmessage = (m) => {
    const e = JSON.parse(m.data);
    if (e.type === "hello") {
      ready = true;
      sendBtn.disabled = busy;
      return;
    }
    if (e.type === "permission") {
      permText.textContent = e.summary || "";
      if (permTitle) {
        if (e.status === "terminal") permTitle.textContent = "Allow terminal command?";
        else if (e.status === "destructive") permTitle.textContent = "Allow risky action?";
        else if (e.kind === "mcp") permTitle.textContent = "Allow MCP tool?";
        else permTitle.textContent = "Allow this change?";
      }
      permEl.classList.add("is-on");
      setThinking(false);
      return;
    }
    if (e.type === "extension_recommend") {
      const existing = extRecsEl?.querySelector('[data-id="' + e.path + '"]');
      if (!existing && extRecsEl) {
        extRecsEl.hidden = false;
        if (!extRecsEl.querySelector(".ext-recs-head")) {
          const head = document.createElement("p");
          head.className = "ext-recs-head";
          head.textContent = "Suggested for this task";
          extRecsEl.appendChild(head);
        }
        extRecsEl.appendChild(buildExtCard(e, { recommend: true }));
      }
      return;
    }
    if (e.type === "extension_installed") {
      if (e.status === "auto") {
        showExtToast(e.text || "Extension installed", e.summary, e.path);
      } else {
        add("system", e.text || "Extension installed");
      }
      refresh();
      return;
    }
    if (e.type === "extension_auth") {
      showAuthPrompt(e.text, e.summary, e.path);
      return;
    }
    if (e.type === "extension_undo") {
      add("system", e.text || "Extension removed");
      refresh();
      return;
    }
    if (e.type === "title") {
      const row = threadsCache.find((s) => s.id === sessionId);
      if (row) row.title = e.text;
      renderThreads();
      return;
    }
    if (e.type === "context") {
      renderContext(e);
      return;
    }
    if (e.type === "review" && e.path) {
      openReview(e.path, e.line || 0, e.line_end || e.line || 0, "Reading file…");
      pushActivity("read", e.path, e.path);
      return;
    }
    if (e.type === "done") {
      permEl.classList.remove("is-on");
      finishTurnUI();
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
      if (e.kind === "read") {
        turnStats.reads = e.count || turnStats.reads + 1;
        if (e.path) pushActivity("read", e.path, e.path);
      }
      if (e.kind === "search") {
        turnStats.searches = e.count || turnStats.searches + 1;
        pushActivity("search", "search");
      }
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
    sendBtn.disabled = true;
    if (statusText.textContent !== "Choose a folder…") {
      statusText.textContent = "Reconnecting…";
    }
  };
}

connectEvents();
syncEmpty();
refresh();
