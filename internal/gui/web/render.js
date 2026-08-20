/* Markdown + LaTeX rendering for assistant messages */

const CODE_REF_RE = /([`']?)((?:[\w.-]+\/)+[\w.-]+\.[a-zA-Z0-9]+)\1:(\d+)(?:-(\d+))?/g;

function escapeHtml(s) {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function protectMath(text) {
  const slots = [];
  const stash = (html) => {
    const i = slots.length;
    slots.push(html);
    return `\x00M${i}\x00`;
  };

  text = text.replace(/\$\$([\s\S]+?)\$\$/g, (_, tex) => {
    try {
      return stash(window.katex.renderToString(tex.trim(), { displayMode: true, throwOnError: false }));
    } catch {
      return stash(`<pre class="math-fallback">${escapeHtml(tex)}</pre>`);
    }
  });
  text = text.replace(/\\\[([\s\S]+?)\\\]/g, (_, tex) => {
    try {
      return stash(window.katex.renderToString(tex.trim(), { displayMode: true, throwOnError: false }));
    } catch {
      return stash(`<pre class="math-fallback">${escapeHtml(tex)}</pre>`);
    }
  });
  text = text.replace(/(?<!\$)\$(?!\$)([^\$\n]+?)\$(?!\$)/g, (_, tex) => {
    try {
      return stash(window.katex.renderToString(tex.trim(), { displayMode: false, throwOnError: false }));
    } catch {
      return stash(`<code class="math-fallback">${escapeHtml(tex)}</code>`);
    }
  });
  text = text.replace(/\\\(([\s\S]+?)\\\)/g, (_, tex) => {
    try {
      return stash(window.katex.renderToString(tex.trim(), { displayMode: false, throwOnError: false }));
    } catch {
      return stash(`<code class="math-fallback">${escapeHtml(tex)}</code>`);
    }
  });
  return { text, slots };
}

function restoreMath(html, slots) {
  return html.replace(/\x00M(\d+)\x00/g, (_, i) => slots[+i] || "");
}

function renderInlineMarkdown(line) {
  let s = escapeHtml(line);
  s = s.replace(/`([^`]+)`/g, "<code>$1</code>");
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  s = s.replace(/\*([^*]+)\*/g, "<em>$1</em>");
  s = s.replace(CODE_REF_RE, (_, _q, path, start, end) => {
    const label = path + ":" + start + (end ? "-" + end : "");
    return `<button type="button" class="code-ref" data-path="${escapeHtml(path)}" data-start="${start}" data-end="${end || start}">${label}</button>`;
  });
  return s;
}

function renderMarkdown(text) {
  const { text: protectedText, slots } = protectMath(text);
  const lines = protectedText.split("\n");
  const out = [];
  let inCode = false;
  let codeLang = "";
  let codeBuf = [];
  let listType = null;
  let para = [];

  const flushPara = () => {
    if (!para.length) return;
    out.push("<p>" + para.map(renderInlineMarkdown).join("<br>") + "</p>");
    para = [];
  };
  const flushList = () => {
    if (!listType) return;
    out.push(`</${listType}>`);
    listType = null;
  };
  const flushCode = () => {
    if (!inCode) return;
    out.push(`<pre><code class="language-${escapeHtml(codeLang)}">${escapeHtml(codeBuf.join("\n"))}</code></pre>`);
    inCode = false;
    codeLang = "";
    codeBuf = [];
  };

  for (const raw of lines) {
    const line = raw;
    if (line.startsWith("```")) {
      flushPara();
      flushList();
      if (!inCode) {
        inCode = true;
        codeLang = line.slice(3).trim();
      } else {
        flushCode();
      }
      continue;
    }
    if (inCode) {
      codeBuf.push(line);
      continue;
    }
    if (/^#{1,6}\s/.test(line)) {
      flushPara();
      flushList();
      const level = line.match(/^#+/)[0].length;
      const title = line.replace(/^#+\s*/, "");
      out.push(`<h${level}>${renderInlineMarkdown(title)}</h${level}>`);
      continue;
    }
    if (/^[-*]\s+/.test(line)) {
      flushPara();
      if (listType !== "ul") {
        flushList();
        out.push("<ul>");
        listType = "ul";
      }
      out.push("<li>" + renderInlineMarkdown(line.replace(/^[-*]\s+/, "")) + "</li>");
      continue;
    }
    if (/^\d+\.\s+/.test(line)) {
      flushPara();
      if (listType !== "ol") {
        flushList();
        out.push("<ol>");
        listType = "ol";
      }
      out.push("<li>" + renderInlineMarkdown(line.replace(/^\d+\.\s+/, "")) + "</li>");
      continue;
    }
    if (line.trim() === "") {
      flushPara();
      flushList();
      continue;
    }
    flushList();
    para.push(line);
  }
  flushPara();
  flushList();
  flushCode();
  return restoreMath(out.join("\n"), slots);
}

function renderContent(text, container, onRefClick) {
  if (!text) {
    container.textContent = "";
    return;
  }
  if (!window.katex) {
    container.textContent = text;
    return;
  }
  container.innerHTML = renderMarkdown(text);
  container.querySelectorAll(".code-ref").forEach((btn) => {
    btn.onclick = () => {
      if (onRefClick) {
        onRefClick(btn.dataset.path, +btn.dataset.start, +btn.dataset.end);
      }
    };
  });
}

window.renderContent = renderContent;
