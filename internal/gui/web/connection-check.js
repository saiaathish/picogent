/* An explicit, metadata-only check. No prompts or API-key fallback. */
(() => {
  const box = document.getElementById("antigravity-check");
  const button = document.getElementById("antigravity-check-btn");
  const status = document.getElementById("antigravity-check-status");
  const details = document.getElementById("antigravity-check-details");
  const list = document.getElementById("antigravity-check-list");
  let provider = "", generation = 0;
  window.AntigravityCheck = {
    afterCheck: null,
    setProvider(value) {
      if (provider !== value) {
        provider = value;
        generation++;
        button.disabled = false;
        button.textContent = "Check connection";
        status.textContent = "Checks your login and model access without generating content.";
        details.hidden = true;
        details.open = false;
        list.replaceChildren();
      }
      box.hidden = value !== "antigravity";
      document.getElementById("login-refresh").hidden = !["codex", "quadcode", "antigravity"].includes(value);
      document.getElementById("login-refresh-status").textContent = "";
      document.getElementById("login-reconnect-btn").hidden = true;
    },
  };
  const refreshBtn = document.getElementById("login-refresh-btn");
  const reconnectBtn = document.getElementById("login-reconnect-btn");
  let reconnectProvider = "";
  refreshBtn.onclick = async () => {
    const selected = provider;
    const output = document.getElementById("login-refresh-status");
    refreshBtn.disabled = true;
    output.textContent = "Refreshing login…";
    try {
      const response = await fetch("/api/setup/refresh", { method:"POST", headers:{"content-type":"application/json"}, body:JSON.stringify({provider:selected}), signal:AbortSignal.timeout(24000) });
      if (!response.ok) throw new Error("Login refresh unavailable");
      const result = await response.json();
      if (selected !== provider) return;
      await window.AntigravityCheck.afterCheck?.();
      output.textContent = result.detail;
      reconnectProvider = selected;
      reconnectBtn.hidden = result.ok;
    } catch (_) { if (selected === provider) { output.textContent = "Login refresh unavailable. Reconnect to sign in again."; reconnectProvider = selected; reconnectBtn.hidden = false; } }
    finally { refreshBtn.disabled = false; }
  };
  reconnectBtn.onclick = async () => {
    const output = document.getElementById("login-refresh-status");
    reconnectBtn.disabled = true;
    try {
      const response = await fetch("/api/setup/login", { method:"POST", headers:{"content-type":"application/json"}, body:JSON.stringify({provider:reconnectProvider,target:reconnectProvider === "quadcode" ? "claude" : reconnectProvider,force:true,return_to:location.href}) });
      if (!response.ok) throw new Error("Reconnect failed");
      const result = await response.json();
      if (result.url) location.href = result.url;
      else output.textContent = result.hint || "Finish login in Terminal, then refresh login here.";
    } catch (_) { output.textContent = "Could not start login. Open Setup and try again."; }
    finally { reconnectBtn.disabled = false; }
  };
  button.onclick = async () => {
    if (provider !== "antigravity" || button.disabled) return;
    const request = ++generation;
    button.disabled = true;
    button.textContent = "Checking…";
    status.textContent = "Checking credentials and Google model access…";
    details.hidden = true;
    try {
      const response = await fetch("/api/setup/check", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ provider: "antigravity" }),
        signal: AbortSignal.timeout(22000),
      });
      if (!response.ok) throw new Error("Connection check could not run. Refresh and try again.");
      const result = await response.json();
      if (request !== generation) return;
      const checks = Array.isArray(result.checks) ? result.checks : [];
      status.textContent = result.ok
        ? `Connection verified · ${result.model_count} models available. Model generation was not tested.`
        : checks.find(check => !check.ok)?.detail || "Connection could not be verified.";
      list.replaceChildren();
      for (const check of checks) {
        const row = document.createElement("li");
        row.textContent = (check.ok ? "✓ " : "• ") + check.detail;
        list.appendChild(row);
      }
      details.hidden = !checks.length;
      await window.AntigravityCheck.afterCheck?.();
    } catch (error) {
      if (request !== generation) return;
      status.textContent = error.name === "TimeoutError"
        ? "The connection check timed out. Try again."
        : "Could not complete the connection check. Check your network, refresh, and retry.";
    } finally {
      if (request === generation) {
        button.disabled = false;
        button.textContent = "Check again";
      }
    }
  };
})();
