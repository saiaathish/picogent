/* Picogent chat UI — storage stays on server (~/.picogent/sessions), not localStorage */

const $ = (id) => document.getElementById(id);

const logEl = $("log");
const emptyEl = $("empty");
const thinkingEl = $("thinking");
const permEl = $("perm");
const permText = $("perm-text");
const permHint = $("perm-hint");
const promptEl = $("prompt");
const sendBtn = $("send");
const attachBtn = $("attach-btn");
const fileInput = $("file-input");
const attachTray = $("attach-tray");
const modelPick = $("model-pick");
const statusText = $("status-text");
const contextBar = $("context-bar");
const contextRing = $("context-ring");
const contextPop = $("context-pop");
const contextPopPct = $("context-pop-pct");
const contextPopTokens = $("context-pop-tokens");
const contextPopNote = $("context-pop-note");
const threadList = $("thread-list");
const shell = $("shell");
const reviewRail = $("rail-review");
const sideRail = $("rail-side");
const reviewPath = $("review-path");
const reviewScroll = $("review-scroll");
const reviewNote = $("review-note");
const reviewInput = $("review-input");
const modeSeg = $("mode-seg");
const slashMenu = $("slash-menu");
const scrim = $("scrim");
const threadSearch = $("thread-search");
const reasoningEl = $("reasoning");
const overviewCard = $("overview-card");
const overviewPct = $("overview-pct");
const overviewBar = $("overview-bar");
const projectList = $("project-list");
const changesList = $("changes-list");
const changesSummary = $("changes-summary");
const activityList = $("activity-list");
const extRecsEl = $("ext-recs");
const extToastsEl = $("ext-toasts");
const permTitle = $("perm-title");
let threadsCache = [];
let chatsOpen = false;
let sideOpen = false;
let sideBusy = false;
let sideStream = null;
let turnChanges = [];
let turnStats = { reads: 0, searches: 0, edits: 0, added: 0, removed: 0 };
let activityItems = [];
let activityPanel = null;

let ready = false;
let busy = false;
let sessionId = "";
let viewEpoch = 0;
let pendingAttachments = [];
let modelOptions = [];
let userModelChoice = "auto";
let slashItems = [];
let slashIndex = 0;
let reviewOpen = false;
let reviewFile = "";
let highlight = { start: 0, end: 0 };

const REF_RE = /([`']?)((?:[\w.-]+\/)+[\w.-]+\.[a-zA-Z0-9]+)\1:(\d+)(?:-(\d+))?/g;
const LINE_RE = /(?:^|\s)(?:line|L)\s*(\d+)(?:\s*[-–]\s*(\d+))?/gi;

function syncEmpty() {
  // Show starter hero (prompts + repo knowledge) whenever the chat log is empty.
  emptyEl.hidden = logEl.children.length > 0;
}

function setThinking(on) {
  thinkingEl.classList.toggle("is-on", !!on);
  $("composer")?.classList.toggle("is-busy", !!on);
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
      html = "Edited " + turnStats.edits + " file" + (turnStats.edits === 1 ? "" : "s");
      if (turnStats.added || turnStats.removed) {
        html += ' <span class="diff-add">+' + turnStats.added + '</span> <span class="diff-del">−' + turnStats.removed + "</span>";
      }
    }
    summary.innerHTML = html || "View changes";
    summary.hidden = !turnStats.edits;
  }
}

function formatTokens(n) {
  const v = Math.max(0, Number(n) || 0);
  if (v >= 1000) {
    const k = v / 1000;
    return "~" + (k >= 100 ? Math.round(k) : k.toFixed(k >= 10 ? 0 : 1)) + "K";
  }
  return String(Math.round(v));
}

function setContextPopOpen(open) {
  if (!contextPop || !contextRing) return;
  contextPop.hidden = !open;
  contextRing.setAttribute("aria-expanded", open ? "true" : "false");
}

function renderContext(ctx) {
  if (!contextBar || !ctx) return;
  const pct = Math.min(100, Math.max(0, Math.round((ctx.pct || 0) * 100)));
  const level = ctx.level || "ok";
  contextBar.hidden = pct <= 0 && level === "ok";
  if (contextBar.hidden) {
    setContextPopOpen(false);
    return;
  }
  contextBar.dataset.level = level;
  const fill = contextBar.querySelector(".context-fill");
  if (fill) fill.style.strokeDasharray = pct + " 100";
  if (contextRing) {
    let label = "Context window " + pct + "% full";
    if (level === "critical") label += ", compacting";
    else if (level === "warning") label += ", approaching limit";
    contextRing.setAttribute("aria-label", label);
    contextRing.title = formatTokens(ctx.tokens) + " / " + formatTokens(ctx.budget) + " tokens";
  }
  if (contextPopPct) contextPopPct.textContent = pct + "% full";
  if (contextPopTokens) {
    contextPopTokens.textContent = formatTokens(ctx.tokens) + " / " + formatTokens(ctx.budget) + " tokens";
  }
  if (contextPopNote) {
    if (level === "critical") {
      contextPopNote.textContent = ctx.status
        ? "Compacting (" + ctx.status + ") to keep room for the next tool rounds."
        : "Compacting automatically to keep room for the next tool rounds.";
    } else if (level === "warning") {
      contextPopNote.textContent = "Getting full — older tool output will be trimmed soon.";
    } else if (ctx.status) {
      contextPopNote.textContent = "Recently compacted (" + ctx.status + "). Soft-fit keeps the ring low on a 256k Codex ceiling.";
    } else {
      contextPopNote.textContent = "256k Codex ceiling — soft compaction keeps the live set small so this grows slowly.";
    }
  }
}

function renderOverview(ov) {
  if (!ov || !ov.knowledge) {
    overviewCard.hidden = true;
    return;
  }
  overviewCard.hidden = false;
  const pct = ov.knowledge || 0;
  let label = pct + "% explored";
  const ev = ov.evolve;
  if (ev && (ev.habits > 0 || ev.playbooks > 0)) {
    label += " · " + (ev.habits || 0) + " habits · " + (ev.playbooks || 0) + " playbooks";
  }
  overviewPct.textContent = label;
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
  if ($("panel-activity")) $("panel-activity").hidden = tab !== "activity";
  if (tab === "activity") refreshActivity();
}

async function refreshActivity() {
  if (!activityList) return;
  try {
    const data = await (await fetch("/api/trace")).json();
    const events = data.events || [];
    if (!events.length) {
      activityList.innerHTML = '<p class="review-empty">No activity yet.</p>';
      return;
    }
    activityList.innerHTML = "";
    for (const ev of events.slice().reverse()) {
      const row = document.createElement("div");
      row.className = "activity-row";
      const kind = ev.kind || "";
      const tool = ev.tool || "";
      const ok = ev.ok === true ? "ok" : ev.ok === false ? "fail" : "";
      const title = document.createElement("strong");
      title.textContent = tool ? kind + " · " + tool : kind;
      const detail = document.createElement("span");
      detail.textContent = (ev.detail || "").slice(0, 160);
      const meta = document.createElement("small");
      meta.textContent = [ev.ts, ok, ev.ms ? ev.ms + "ms" : ""].filter(Boolean).join(" · ");
      row.appendChild(title);
      if (detail.textContent) row.appendChild(detail);
      row.appendChild(meta);
      activityList.appendChild(row);
    }
  } catch (err) {
    activityList.innerHTML = '<p class="review-empty">Could not load activity.</p>';
  }
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
  try {
    const res = await fetch("/api/projects", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "pick" }),
    });
    if (res.status === 204) {
      return;
    }
    if (!res.ok) {
      add("error", await res.text());
      return;
    }
    await applyProjectSwitch(await res.json());
  } catch (err) {
    add("error", err.message || "Couldn't open folder picker");
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
  const on = chatsOpen || reviewOpen || sideOpen;
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
  if (on) setSideOpen(false);
  syncScrim();
}

function setSideOpen(on) {
  sideOpen = on;
  shell.classList.toggle("side-open", on);
  if (sideRail) sideRail.hidden = !on;
  if (on) {
    setReviewOpen(false);
    loadSidePrompts(false);
    $("side-input")?.focus();
  }
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

/* Live / typewriter assistant streaming */
let stream = null;

function ensureStreamBubble() {
  if (stream?.content?.isConnected) return stream;
  const wrap = document.createElement("div");
  wrap.className = "turn turn-assistant is-streaming";
  const c = document.createElement("div");
  c.className = "content md";
  wrap.appendChild(c);
  logEl.appendChild(wrap);
  syncEmpty();
  stream = { wrap, content: c, target: "", shown: 0, timer: 0, live: false, finishing: false };
  return stream;
}

function paintStream() {
  if (!stream) return;
  const shown = stream.target.slice(0, stream.shown);
  const showCaret = stream.shown < stream.target.length || stream.live;
  if (window.renderStreamingContent && showCaret) {
    window.renderStreamingContent(shown, stream.content, (path, start, end) => {
      openReview(path, start, end, "Referenced in reply");
    });
  } else if (window.renderContent) {
    window.renderContent(shown, stream.content, (path, start, end) => {
      openReview(path, start, end, "Referenced in reply");
    });
    if (showCaret) {
      const caret = document.createElement("span");
      caret.className = "stream-cursor";
      caret.setAttribute("aria-hidden", "true");
      stream.content.appendChild(caret);
    }
  } else {
    stream.content.textContent = "";
    stream.content.appendChild(document.createTextNode(shown));
    if (showCaret) {
      const caret = document.createElement("span");
      caret.className = "stream-cursor";
      caret.setAttribute("aria-hidden", "true");
      stream.content.appendChild(caret);
    }
  }
}

function stopStreamTimer() {
  if (stream?.timer) {
    clearTimeout(stream.timer);
    stream.timer = 0;
  }
}

function kickTypewriter() {
  if (!stream || stream.timer) return;
  const tick = () => {
    if (!stream) return;
    stream.timer = 0;
    const lag = stream.target.length - stream.shown;
    if (lag <= 0) {
      if (stream.finishing) finalizeStream();
      return;
    }
    // ~35–50 chars/sec; catch up faster when behind (live stream)
    let step = 1;
    if (stream.live) step = Math.min(lag, lag > 120 ? 24 : lag > 40 ? 8 : 3);
    else step = Math.min(lag, lag > 100 ? 6 : lag > 30 ? 3 : 1);
    stream.shown += step;
    paintStream();
    scrollChat();
    const delay = stream.live ? 10 : 18;
    stream.timer = setTimeout(tick, delay);
  };
  stream.timer = setTimeout(tick, 12);
}

function appendAssistantDelta(delta) {
  if (!delta) return;
  const s = ensureStreamBubble();
  s.live = true;
  s.target += delta;
  thinkingEl.classList.remove("is-on");
  kickTypewriter();
}

function typeAssistantFull(text) {
  if (!text) {
    finalizeStream();
    return;
  }
  const s = ensureStreamBubble();
  s.live = false;
  s.target = text;
  s.shown = Math.min(s.shown, text.length);
  s.finishing = true;
  thinkingEl.classList.remove("is-on");
  kickTypewriter();
}

function finalizeStream() {
  if (!stream) return;
  stopStreamTimer();
  const text = stream.target;
  const content = stream.content;
  const wrap = stream.wrap;
  stream = null;
  if (!text) {
    wrap.remove();
    syncEmpty();
    return;
  }
  linkifyRefs(text, content);
  applyRefsFromText(text);
  wrap.classList.remove("is-streaming");
  scrollChat();
}

function add(kind, text, attachmentMeta) {
  if (kind === "tool") {
    return;
  }

  const wrap = document.createElement("div");
  const role = kind === "you" || kind === "user" ? "user" : kind;
  wrap.className = "turn turn-" + role;

  if (role === "user") {
    if (attachmentMeta?.length) {
      const tray = document.createElement("div");
      tray.className = "msg-attachments";
      for (const a of attachmentMeta) {
        const chip = document.createElement("span");
        chip.className = "msg-attach-chip";
        if (a.preview) {
          const img = document.createElement("img");
          img.src = a.preview;
          img.alt = a.name;
          chip.appendChild(img);
        } else {
          chip.textContent = a.name;
        }
        tray.appendChild(chip);
      }
      wrap.appendChild(tray);
    }
    const b = document.createElement("div");
    b.className = "bubble";
    b.textContent = text;
    wrap.appendChild(b);
  } else if (role === "error") {
    const b = document.createElement("div");
    b.className = "bubble bubble-error";
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
  stopStreamTimer();
  stream = null;
  logEl.innerHTML = "";
  resetReasoning();
  setThinking(false);
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
  viewEpoch++;
  const epoch = viewEpoch;
  const data = await (
    await fetch("/api/sessions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "load", id }),
    })
  ).json();
  if (epoch !== viewEpoch) return;
  sessionId = data.id;
  replayMessages(data.messages || []);
  setChatsOpen(false);
  await loadThreads();
}

async function newChat() {
  viewEpoch++;
  const epoch = viewEpoch;
  setChatsOpen(false);
  if (busy) {
    try { await fetch("/api/cancel", { method: "POST" }); } catch (_) {}
  }
  const data = await (
    await fetch("/api/sessions", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ action: "new" }),
    })
  ).json();
  if (epoch !== viewEpoch) return;
  sessionId = data.id;
  clearLog();
  // Starter hero is inside #empty — force it visible before prompts paint.
  emptyEl.hidden = false;
  await loadThreads();
  loadHeroPrompts(true);
  // Re-pull overview / auth so repo knowledge shows on a fresh chat.
  try {
    const s = await (await fetch("/api/state")).json();
    if (epoch !== viewEpoch) return;
    renderOverview({ ...(s.overview || {}), evolve: s.evolve });
    renderAuthBanner(s.auth);
  } catch (_) {}
  loadSideChat();
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
  const epoch = viewEpoch;
  const s = await (await fetch("/api/state")).json();
  if (epoch !== viewEpoch) return;
  sessionId = s.session_id || sessionId;
  currentMode = s.mode || "safe";
  if (s.slash?.length) slashItems = s.slash;

  modeSeg.querySelectorAll("button").forEach((b) => {
    b.classList.toggle("is-on", b.dataset.mode === s.mode);
  });

  if (s.model_options?.length) {
    if (s.model === "auto") userModelChoice = "auto";
    fillModelPick(s.model_options, userModelChoice);
  }

  renderAuthBanner(s.auth);

  renderOverview({
    ...(s.overview || {}),
    evolve: s.evolve,
  });
  renderContext(s.context);
  if (s.pending_perm) {
    showPermission(s.pending_perm);
  } else if (!s.busy) {
    permEl.classList.remove("is-on");
  }
  if (logEl.children.length === 0 && s.messages?.length) {
    replayMessages(s.messages);
  }
  busy = !!s.busy;
  sendBtn.disabled = !ready || !!s.auth?.needed;
  setThinking(busy);
  syncEmpty();
  await loadThreads();
  await loadProjects();
}

let authPollTimer = null;
let lastAuthTarget = "";

function renderAuthBanner(auth) {
  const banner = $("auth-banner");
  if (!banner) return;
  if (!auth?.needed) {
    banner.hidden = true;
    if (authPollTimer) {
      clearInterval(authPollTimer);
      authPollTimer = null;
    }
    return;
  }
  banner.hidden = false;
  lastAuthTarget = auth.target || "";
  $("auth-banner-title").textContent = auth.label
    ? "Log in to " + auth.label
    : "Log in to continue";
  $("auth-banner-detail").textContent = auth.detail || "Connect your provider to chat.";
  const btn = $("auth-banner-btn");
  btn.textContent = auth.button || "Log in";
  btn.disabled = false;
}

async function startProviderLogin(target, statusEl, btn) {
  if (!target) {
    location.href = "/settings.html";
    return;
  }
  if (target === "settings") {
    location.href = "/settings.html";
    return;
  }
  if (btn) {
    btn.disabled = true;
    btn.textContent = "Opening…";
  }
  if (statusEl) {
    statusEl.hidden = false;
    statusEl.textContent = "Starting login…";
  }
  try {
    const res = await fetch("/api/setup/login", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        target,
        return_to: location.origin + "/",
      }),
    });
    const text = await res.text();
    let data = {};
    try {
      data = JSON.parse(text);
    } catch {
      if (!res.ok) throw new Error(text || "Login failed");
    }
    if (!res.ok) {
      throw new Error(data.error || text || "Login failed");
    }
    if (data.url) {
      location.href = data.url;
      return;
    }
    if (statusEl) {
      statusEl.hidden = false;
      statusEl.textContent = data.hint || "Finish login in the window that opened, then come back here.";
    }
    if (btn) {
      btn.disabled = false;
      btn.textContent = "Try again";
    }
    if (authPollTimer) clearInterval(authPollTimer);
    authPollTimer = setInterval(async () => {
      const s = await (await fetch("/api/state")).json();
      if (!s.auth?.needed) {
        clearInterval(authPollTimer);
        authPollTimer = null;
        renderAuthBanner(null);
        sendBtn.disabled = !ready;
        if (statusEl) {
          statusEl.hidden = false;
          statusEl.textContent = "Connected — you’re ready to chat.";
        }
      }
    }, 2000);
  } catch (err) {
    if (statusEl) {
      statusEl.hidden = false;
      statusEl.textContent = err.message || String(err);
    }
    if (btn) {
      btn.disabled = false;
      btn.textContent = "Log in";
    }
  }
}

$("auth-banner-btn")?.addEventListener("click", () => {
  startProviderLogin(lastAuthTarget, $("auth-banner-status"), $("auth-banner-btn"));
});


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

function bindNewChat(el) {
  if (!el) return;
  el.addEventListener("click", (e) => {
    e.preventDefault();
    newChat();
  });
}
bindNewChat($("new-chat"));
bindNewChat($("new-chat-top"));

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
  toast.innerHTML = "<strong>Authorize " + escapeHtml(name) + "</strong><p>" + escapeHtml(hint || "Add credentials in Settings → Extensions") + "</p>";
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

function fillModelPick(options, selected) {
  modelOptions = options || [];
  if (!modelPick) return;
  modelPick.innerHTML = "";
  for (const opt of modelOptions) {
    const o = document.createElement("option");
    o.value = opt.value;
    o.textContent = opt.label;
    o.title = opt.description || "";
    modelPick.appendChild(o);
  }
  const pick = userModelChoice === "auto" ? "auto" : (selected || userModelChoice || "auto");
  if ([...modelPick.options].some((o) => o.value === pick)) {
    modelPick.value = pick;
  } else if (modelPick.options.length) {
    modelPick.value = modelPick.options[0].value;
  }
}

modelPick?.addEventListener("change", async () => {
  userModelChoice = modelPick.value;
  await fetch("/api/settings", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ model: modelPick.value }),
  });
  refresh();
});

const MAX_ATTACH = 8;
const MAX_ATTACH_BYTES = 10 * 1024 * 1024;

function renderAttachTray() {
  if (!attachTray) return;
  attachTray.innerHTML = "";
  attachTray.hidden = pendingAttachments.length === 0;
  for (const a of pendingAttachments) {
    const chip = document.createElement("div");
    chip.className = "attach-chip";
    if (a.preview) {
      const img = document.createElement("img");
      img.src = a.preview;
      img.alt = a.name;
      chip.appendChild(img);
    }
    const label = document.createElement("span");
    label.textContent = a.name;
    const rm = document.createElement("button");
    rm.type = "button";
    rm.setAttribute("aria-label", "Remove");
    rm.textContent = "×";
    rm.onclick = () => {
      pendingAttachments = pendingAttachments.filter((x) => x.id !== a.id);
      renderAttachTray();
    };
    chip.appendChild(label);
    chip.appendChild(rm);
    attachTray.appendChild(chip);
  }
}

function fileToAttachment(file) {
  return new Promise((resolve, reject) => {
    if (file.size > MAX_ATTACH_BYTES) {
      reject(new Error(file.name + " is too large (max 10 MB)"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      const dataUrl = reader.result;
      const base64 = String(dataUrl).split(",")[1] || "";
      const mime = file.type || guessMime(file.name);
      const isImage = mime.startsWith("image/");
      resolve({
        id: file.name + "-" + file.size + "-" + Date.now(),
        name: file.name,
        mime,
        data: base64,
        preview: isImage ? dataUrl : "",
      });
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

function guessMime(name) {
  const ext = name.toLowerCase().split(".").pop();
  const map = {
    png: "image/png", jpg: "image/jpeg", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp",
    pdf: "application/pdf", md: "text/markdown", json: "application/json", csv: "text/csv",
  };
  return map[ext] || "text/plain";
}

async function addFiles(fileList) {
  const files = [...fileList];
  if (!files.length) return;
  if (pendingAttachments.length + files.length > MAX_ATTACH) {
    add("error", "Max " + MAX_ATTACH + " attachments per message");
    return;
  }
  for (const file of files) {
    try {
      pendingAttachments.push(await fileToAttachment(file));
    } catch (err) {
      add("error", err.message || "Couldn't read file");
    }
  }
  renderAttachTray();
}

attachBtn?.addEventListener("click", async () => {
  if (busy) return;
  try {
    const res = await fetch("/api/files/pick", { method: "POST" });
    if (res.status === 204) return;
    if (!res.ok) {
      fileInput?.click();
      return;
    }
    const data = await res.json();
    for (const f of data.files || []) {
      if (pendingAttachments.length >= MAX_ATTACH) break;
      const isImage = (f.mime || "").startsWith("image/");
      pendingAttachments.push({
        id: f.name + "-" + Date.now(),
        name: f.name,
        mime: f.mime,
        data: f.data,
        preview: isImage ? "data:" + f.mime + ";base64," + f.data : "",
      });
    }
    renderAttachTray();
  } catch {
    fileInput?.click();
  }
});

fileInput?.addEventListener("change", () => {
  if (fileInput.files?.length) addFiles(fileInput.files);
  fileInput.value = "";
});

promptEl?.addEventListener("paste", (e) => {
  const items = e.clipboardData?.items;
  if (!items) return;
  const files = [];
  for (const item of items) {
    if (item.kind === "file") {
      const f = item.getAsFile();
      if (f) files.push(f);
    }
  }
  if (files.length) {
    e.preventDefault();
    addFiles(files);
  }
});

$("composer").onsubmit = async (e) => {
  e.preventDefault();
  const prompt = promptEl.value.trim();
  if ((!prompt && !pendingAttachments.length) || !ready) return;
  if (busy) {
    // Allow steering / follow-ups while the agent works.
  }
  const sentAttachments = pendingAttachments.map((a) => ({
    name: a.name,
    mime: a.mime,
    data: a.data,
    preview: a.preview,
  }));
  add("you", prompt || "(attached files)", sentAttachments);
  const attachments = pendingAttachments.map((a) => ({ name: a.name, mime: a.mime, data: a.data }));
  pendingAttachments = [];
  renderAttachTray();
  promptEl.value = "";
  promptEl.style.height = "auto";
  const wasBusy = busy;
  if (!wasBusy) {
    setThinking(true);
  }
  sendBtn.disabled = !ready;
  const res = await fetch("/api/chat", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ prompt, attachments }),
  });
  if (res.status === 204) {
    if (!wasBusy) {
      busy = false;
      sendBtn.disabled = !ready;
      setThinking(false);
    }
    return;
  }
  let queued = false;
  if (res.ok) {
    try {
      const data = await res.json();
      queued = !!data.queued;
    } catch (_) {}
  }
  if (queued) {
    return;
  }
  if (!res.ok) {
    add("error", await res.text());
    if (!wasBusy) {
      busy = false;
      sendBtn.disabled = !ready;
      setThinking(false);
    }
  }
};

promptEl.addEventListener("keydown", (e) => {
  if (slashMenuOpen()) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      slashIndex = Math.min(slashIndex + 1, visibleSlash().length - 1);
      renderSlashMenu();
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      slashIndex = Math.max(slashIndex - 1, 0);
      renderSlashMenu();
      return;
    }
    if (e.key === "Enter" || e.key === "Tab") {
      const pick = visibleSlash()[slashIndex];
      if (pick) {
        e.preventDefault();
        applySlash(pick);
        return;
      }
    }
    if (e.key === "Escape") {
      hideSlashMenu();
      return;
    }
  }
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    $("composer").requestSubmit();
  }
});

promptEl.addEventListener("input", () => {
  promptEl.style.height = "auto";
  promptEl.style.height = Math.min(promptEl.scrollHeight, 160) + "px";
  updateSlashMenu();
});

const DEFAULT_SLASH = [
  { name: "commit", hint: "Commit current changes" },
  { name: "review", hint: "Review uncommitted diffs" },
  { name: "status", hint: "Mode, model, workspace" },
  { name: "diff", hint: "Show git diff" },
  { name: "compact", hint: "Shrink context" },
  { name: "memory", hint: "Project rules + what Picogent learned" },
  { name: "goal", hint: "Show, set, or clear goal", insert: "/goal " },
  { name: "agent", hint: "Default agent task mode" },
  { name: "ask", hint: "Answer without editing" },
  { name: "plan", hint: "Plan before building" },
  { name: "debug", hint: "Investigate a bug" },
  { name: "clear", hint: "New chat" },
];

function slashCatalog() {
  return slashItems.length ? slashItems : DEFAULT_SLASH;
}

function slashQuery() {
  const v = promptEl.value;
  if (!v.startsWith("/")) return null;
  if (v.includes(" ") || v.includes("\n")) return null;
  return v.slice(1).toLowerCase();
}

function visibleSlash() {
  const q = slashQuery();
  if (q === null) return [];
  return slashCatalog().filter((it) => {
    const name = (it.name || "").toLowerCase();
    return name.startsWith(q);
  });
}

function slashMenuOpen() {
  return slashMenu && !slashMenu.hidden;
}

function hideSlashMenu() {
  if (!slashMenu) return;
  slashMenu.hidden = true;
  slashMenu.innerHTML = "";
}

function updateSlashMenu() {
  const items = visibleSlash();
  if (!items.length) {
    hideSlashMenu();
    return;
  }
  if (slashIndex >= items.length) slashIndex = 0;
  renderSlashMenu();
}

function renderSlashMenu() {
  const items = visibleSlash();
  if (!slashMenu || !items.length) {
    hideSlashMenu();
    return;
  }
  slashMenu.hidden = false;
  slashMenu.innerHTML = "";
  items.forEach((it, i) => {
    const b = document.createElement("button");
    b.type = "button";
    b.className = "slash-item" + (i === slashIndex ? " is-on" : "");
    b.setAttribute("role", "option");
    b.innerHTML =
      '<span class="slash-name">/' + escapeHtml(it.name) + "</span>" +
      '<span class="slash-hint">' + escapeHtml(it.hint || "") + "</span>";
    b.onmousedown = (e) => {
      e.preventDefault();
      applySlash(it);
    };
    slashMenu.appendChild(b);
  });
}

function applySlash(it) {
  const insert = it.insert || "/" + it.name;
  promptEl.value = insert;
  promptEl.focus();
  promptEl.dispatchEvent(new Event("input"));
  hideSlashMenu();
  if (!insert.endsWith(" ")) {
    $("composer").requestSubmit();
  }
}

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
$("side-fab")?.addEventListener("click", () => setSideOpen(!sideOpen));
$("close-side")?.addEventListener("click", () => setSideOpen(false));
contextRing?.addEventListener("click", (e) => {
  e.stopPropagation();
  if (!contextPop) return;
  setContextPopOpen(contextPop.hidden);
});
document.addEventListener("click", (e) => {
  if (!contextBar || !contextPop || contextPop.hidden) return;
  if (contextBar.contains(e.target)) return;
  setContextPopOpen(false);
});
scrim.onclick = () => {
  setChatsOpen(false);
  setReviewOpen(false);
  setSideOpen(false);
  setContextPopOpen(false);
};
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    setChatsOpen(false);
    setReviewOpen(false);
    setSideOpen(false);
    setContextPopOpen(false);
  }
});

function finishTurnUI() {
  busy = false;
  sendBtn.disabled = !ready;
  if (stream) {
    stream.finishing = true;
    stream.live = false;
    if (stream.shown >= stream.target.length) finalizeStream();
    else kickTypewriter();
  }
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
function showPermission(e) {
  if (!e) return;
  permText.textContent = e.summary || e.text || "";
  if (permHint) {
    const hint = e.hint || "";
    permHint.textContent = hint;
    permHint.hidden = !hint;
  }
  if (permTitle) {
    if (e.status === "terminal") permTitle.textContent = "Allow terminal command?";
    else if (e.status === "destructive") permTitle.textContent = "Allow risky action?";
    else if (e.kind === "mcp") permTitle.textContent = "Allow MCP tool?";
    else permTitle.textContent = "Allow this change?";
  }
  permEl.classList.add("is-on");
  setThinking(false);
}

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
      sendBtn.disabled = !ready;
      return;
    }
    if (e.type === "permission") {
      showPermission(e);
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
      if (permHint) {
        permHint.hidden = true;
        permHint.textContent = "";
      }
      finishTurnUI();
      if ($("panel-activity") && !$("panel-activity").hidden) refreshActivity();
      if (sideOpen) refreshSideBusy();
      return;
    }
    if (e.type === "side_delta") {
      appendSideDelta(e.text || "");
      return;
    }
    if (e.type === "side") {
      finalizeSide(e.text || "");
      return;
    }
    if (e.type === "side_done") {
      if (sideStream) finalizeSide(sideStream.text || "");
      else setSideBusyUI(false);
      return;
    }
    if (e.type === "prompts_refresh") {
      const kind = e.text || "all";
      if (kind === "main" || kind === "all") loadHeroPrompts(true);
      if ((kind === "side" || kind === "all") && sideOpen) loadSidePrompts(true);
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
    if (e.type === "evolve") {
      add("system", e.text || "Picogent remembered something for this folder.");
      return;
    }
    if (e.type === "overview") {
      fetch("/api/overview")
        .then((r) => r.json())
        .then(renderOverview);
      return;
    }
    if (e.type === "route") {
      refresh();
      return;
    }
    if (e.type === "goal") {
      refresh();
      return;
    }
    if (e.type === "task_mode") {
      refresh();
      return;
    }
    if (e.type === "system") {
      add("system", e.text || "");
      return;
    }
    if (e.type === "tool") {
      return;
    }
    if (e.type === "assistant_delta") {
      appendAssistantDelta(e.text || "");
      return;
    }
    if (e.type === "assistant") {
      if (stream?.live) {
        if (e.text && e.text.length >= stream.target.length) stream.target = e.text;
        stream.finishing = true;
        stream.live = false;
        if (stream.shown >= stream.target.length) finalizeStream();
        else kickTypewriter();
      } else if (e.text) {
        typeAssistantFull(e.text);
      } else {
        finalizeStream();
      }
      return;
    }
    add(e.type === "you" ? "you" : e.type, e.text || e.summary || e.type);
    if (busy && !reasoningEl.querySelector(".reason-thinking") && !stream) {
      const t = document.createElement("div");
      t.className = "reason-thinking";
      t.textContent = "Working";
      reasoningEl.appendChild(t);
      scrollChat();
    }
  };
  ev.onerror = () => {
    ready = false;
    sendBtn.disabled = true;
  };
}

connectEvents();
syncEmpty();
refresh().then(() => {
  loadHeroPrompts(false);
});
loadSideChat();

/* ─── PicoChat Companion + AI prompt recommendations ─── */
function setSideBusyUI(on) {
  sideBusy = !!on;
  const form = $("side-ask");
  form?.classList.toggle("is-busy", sideBusy);
  const send = form?.querySelector("button");
  if (send) send.disabled = sideBusy;
}

function refreshSideBusy() {
  /* status widget removed — only keep send/beam in sync after main turns */
}

async function loadSideChat() {
  try {
    const res = await fetch("/api/sidechat");
    const data = await res.json();
    renderSidePromptChips(data.prompts || []);
    const log = $("side-log");
    if (log) {
      log.innerHTML = "";
      (data.messages || []).forEach((m) => addSideBubble(m.role === "user" ? "you" : "assistant", m.text || ""));
      if (!log.children.length) {
        addSideBubble(
          "assistant",
          "Ask for anything about the project or help using Picogent."
        );
      }
    }
  } catch {
    /* ignore */
  }
}

async function loadSidePrompts(force) {
  const row = $("side-starters");
  if (row && force) {
    row.innerHTML = '<button type="button" class="side-chip is-loading">Updating…</button>';
  }
  try {
    const res = await fetch("/api/prompts?kind=side" + (force ? "&refresh=1" : ""));
    const data = await res.json();
    renderSidePromptChips(data.prompts || []);
  } catch {
    if (row && !row.children.length) {
      row.innerHTML = "";
    }
  }
}

function renderSidePromptChips(items) {
  const row = $("side-starters");
  if (!row) return;
  row.innerHTML = "";
  (items || []).forEach((it) => {
    const title = it.title || it.prompt || "";
    const prompt = it.prompt || it.title || "";
    if (!prompt) return;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "side-chip";
    b.textContent = title;
    b.title = it.subtitle || prompt;
    b.onclick = () => askSide(prompt);
    row.appendChild(b);
  });
}

async function loadHeroPrompts(force) {
  const host = $("hero-recs");
  if (!host) return;
  const folder = host.querySelector(".rec-folder");
  // Keep folder button; replace AI cards.
  host.querySelectorAll(".rec.is-ai, .rec.is-loading").forEach((n) => n.remove());
  const loading = document.createElement("button");
  loading.type = "button";
  loading.className = "rec is-loading is-ai";
  loading.innerHTML = "<span>Recommended</span><small>Tuning to this repo…</small>";
  host.appendChild(loading);
  try {
    const res = await fetch("/api/prompts?kind=main" + (force ? "&refresh=1" : ""));
    const data = await res.json();
    loading.remove();
    renderHeroPrompts(data.prompts || [], folder);
  } catch {
    loading.remove();
  }
}

function renderHeroPrompts(items, folderBtn) {
  const host = $("hero-recs");
  if (!host) return;
  host.querySelectorAll(".rec.is-ai").forEach((n) => n.remove());
  const folder = folderBtn || host.querySelector(".rec-folder");
  (items || []).slice(0, 4).forEach((it) => {
    const prompt = it.prompt || "";
    if (!prompt) return;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "rec is-ai";
    b.dataset.prompt = prompt;
    b.innerHTML =
      "<span>" + escapeHtml(it.title || "Try this") + "</span>" +
      "<small>" + escapeHtml(it.subtitle || prompt) + "</small>";
    b.onclick = () => {
      promptEl.value = prompt;
      promptEl.focus();
      promptEl.dispatchEvent(new Event("input"));
    };
    host.appendChild(b);
  });
  if (folder) host.insertBefore(folder, host.firstChild);
}

function addSideBubble(role, text) {
  const log = $("side-log");
  if (!log || !text) return null;
  const wrap = document.createElement("div");
  wrap.className = "side-bubble side-" + role;
  const body = document.createElement("div");
  body.className = "content md";
  if (role === "assistant" && window.renderContent) {
    window.renderContent(text, body);
  } else {
    body.textContent = text;
  }
  wrap.appendChild(body);
  log.appendChild(wrap);
  log.scrollTop = log.scrollHeight;
  return { wrap, body };
}

function ensureSideStream() {
  if (sideStream?.body?.isConnected) return sideStream;
  const bubble = addSideBubble("assistant", "…");
  if (!bubble) return null;
  bubble.body.textContent = "";
  sideStream = { body: bubble.body, text: "" };
  return sideStream;
}

function appendSideDelta(delta) {
  const s = ensureSideStream();
  if (!s || !delta) return;
  s.text += delta;
  if (window.renderStreamingContent) {
    window.renderStreamingContent(s.text, s.body);
  } else if (window.renderContent) {
    window.renderContent(s.text, s.body);
  } else {
    s.body.textContent = s.text;
  }
  const log = $("side-log");
  if (log) log.scrollTop = log.scrollHeight;
}

function finalizeSide(text) {
  const finalText = text || sideStream?.text || "";
  if (sideStream?.body) {
    if (window.renderContent) window.renderContent(finalText, sideStream.body);
    else sideStream.body.textContent = finalText;
  } else if (finalText) {
    addSideBubble("assistant", finalText);
  }
  sideStream = null;
  setSideBusyUI(false);
}

async function askSide(question) {
  const q = (question || "").trim();
  if (!q || sideBusy) return;
  addSideBubble("you", q);
  setSideBusyUI(true);
  try {
    const res = await fetch("/api/sidechat", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ prompt: q }),
    });
    if (!res.ok) {
      finalizeSide("PicoChat is busy or unavailable.");
    }
  } catch {
    finalizeSide("Couldn’t reach PicoChat.");
  }
}

$("side-ask")?.addEventListener("submit", (e) => {
  e.preventDefault();
  const input = $("side-input");
  const q = input?.value.trim();
  if (!q) return;
  input.value = "";
  askSide(q);
});
