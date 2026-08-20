async function load() {
  const s = await (await fetch("/api/settings")).json();
  document.getElementById("workspace").value = s.workspace || "";
  document.getElementById("mode").value = s.mode || "safe";
  document.getElementById("model").value = s.model || "";

  const conn = document.getElementById("connection");
  const connected = s.codex || s.has_api_key;
  if (connected) {
    conn.innerHTML = "Connected via ChatGPT. Need to reconnect? <a href=\"/setup.html\">Open setup</a>.";
  } else {
    conn.innerHTML = "Not connected yet. <a href=\"/setup.html\">Finish setup</a> to log in.";
  }
}

document.getElementById("form").onsubmit = async (e) => {
  e.preventDefault();
  const status = document.getElementById("status");
  const save = document.getElementById("save");
  status.textContent = "Saving…";
  save.disabled = true;

  const body = {
    workspace: document.getElementById("workspace").value,
    mode: document.getElementById("mode").value,
    model: document.getElementById("model").value,
  };

  const res = await fetch("/api/settings", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });

  save.disabled = false;
  status.textContent = res.ok ? "Saved." : "Couldn’t save — try again.";
  if (res.ok) setTimeout(() => (status.textContent = ""), 2000);
};

load();
