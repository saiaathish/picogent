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
    },
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
