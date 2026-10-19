const DEFAULTS = {
  apiBase: "http://127.0.0.1:7501",
  sessionId: "local",
  token: "",
  enabled: true,
};

async function config() {
  const stored = await chrome.storage.local.get(DEFAULTS);
  return { ...DEFAULTS, ...stored };
}

function headers(cfg) {
  const result = { "content-type": "application/json" };
  if (cfg.token) result["X-Api-Token"] = cfg.token;
  return result;
}

async function remember(patch) {
  const current = await chrome.storage.local.get({ status: {} });
  await chrome.storage.local.set({ status: { ...current.status, ...patch, at: new Date().toISOString() } });
}

async function tick() {
  const cfg = await config();
  if (!cfg.enabled) return;
  const [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  if (!tab || !tab.id || !/^https?:\/\//.test(tab.url || "")) {
    await remember({ connected: false, detail: "No active http(s) tab." });
    return;
  }
  let snapshot;
  try {
    snapshot = await chrome.tabs.sendMessage(tab.id, { type: "snapshot" });
  } catch (error) {
    await remember({ connected: false, detail: "Reload the tab so the content script can attach." });
    return;
  }
  try {
    const posted = await fetch(cfg.apiBase.replace(/\/$/, "") + "/v1/extension/snapshot", {
      method: "POST",
      headers: headers(cfg),
      body: JSON.stringify({ sessionId: cfg.sessionId, tabId: tab.id, ...snapshot }),
    });
    if (!posted.ok) throw new Error("snapshot " + posted.status);
    const commandsResponse = await fetch(
      cfg.apiBase.replace(/\/$/, "") + "/v1/extension/commands?sessionId=" + encodeURIComponent(cfg.sessionId),
      { headers: headers(cfg) },
    );
    if (!commandsResponse.ok) throw new Error("commands " + commandsResponse.status);
    const { commands } = await commandsResponse.json();
    let last = "Watching " + (snapshot.title || tab.url);
    for (const command of commands || []) {
      const targetId = command.tabId || tab.id;
      const result = await chrome.tabs.sendMessage(targetId, { type: "execute", command });
      await fetch(cfg.apiBase.replace(/\/$/, "") + "/v1/extension/result", {
        method: "POST",
        headers: headers(cfg),
        body: JSON.stringify({
          sessionId: cfg.sessionId,
          commandId: command.id,
          ok: Boolean(result?.ok),
          detail: result?.detail || "No detail.",
        }),
      });
      last = result?.detail || last;
    }
    await remember({ connected: true, detail: last, elements: (snapshot.elements || []).length });
  } catch (error) {
    await remember({ connected: false, detail: String(error.message || error) });
  }
}

chrome.runtime.onInstalled.addListener(async () => {
  const stored = await chrome.storage.local.get(null);
  await chrome.storage.local.set({ ...DEFAULTS, ...stored });
  chrome.alarms.create("tick", { periodInMinutes: 0.5 });
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "tick") tick();
});

chrome.tabs.onActivated.addListener(() => tick());
chrome.tabs.onUpdated.addListener((_id, info) => {
  if (info.status === "complete") tick();
});
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === "tick") {
    tick().then(() => sendResponse({ ok: true }));
    return true;
  }
  return false;
});

setInterval(tick, 1000);
tick();
