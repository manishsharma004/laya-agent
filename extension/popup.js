const api = document.getElementById("api");
const session = document.getElementById("session");
const token = document.getElementById("token");
const enabled = document.getElementById("enabled");
const pill = document.getElementById("pill");
const detail = document.getElementById("detail");

async function load() {
  const stored = await chrome.storage.local.get({
    apiBase: "http://127.0.0.1:7501",
    sessionId: "local",
    token: "",
    enabled: true,
    status: {},
  });
  api.value = stored.apiBase;
  session.value = stored.sessionId;
  token.value = stored.token;
  enabled.checked = stored.enabled;
  pill.textContent = stored.status.connected ? "live" : "idle";
  pill.classList.toggle("on", Boolean(stored.status.connected));
  detail.textContent = stored.status.detail || "Waiting for the active tab.";
}

document.getElementById("save").addEventListener("click", async () => {
  await chrome.storage.local.set({
    apiBase: api.value.trim() || "http://127.0.0.1:7501",
    sessionId: session.value.trim() || "local",
    token: token.value,
    enabled: enabled.checked,
  });
  chrome.runtime.sendMessage({ type: "tick" });
  detail.textContent = "Saved.";
});

load();
