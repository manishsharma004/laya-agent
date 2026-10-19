const DEFAULTS = {
  apiBase: "http://127.0.0.1:7505",
  sessionId: "local",
  token: "",
  enabled: true,
  availableTabIds: null,
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

function tabUrl(tab) {
  return tab?.url || tab?.pendingUrl || "";
}

async function tick() {
  const cfg = await config();
  if (!cfg.enabled) return;
  const tabs = await chrome.tabs.query({});
  const [active] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  const known = cfg.availableTabIds;
  const reports = [];
  for (const tab of tabs) {
    const url = tabUrl(tab);
    if (!tab.id || !/^https?:\/\//.test(url)) continue;
    const available = known == null || known.includes(tab.id);
    let elements = [];
    if (available) {
      try {
        const snapshot = await chrome.tabs.sendMessage(tab.id, { type: "snapshot" });
        elements = snapshot?.elements || [];
      } catch {
        elements = [];
      }
    }
    reports.push({
      tabId: tab.id,
      windowId: tab.windowId,
      url,
      title: tab.title || "",
      active: tab.id === active?.id,
      elements,
    });
  }
  const base = cfg.apiBase.replace(/\/$/, "");
  try {
    const posted = await fetch(base + "/v1/extension/tabs", {
      method: "POST",
      headers: headers(cfg),
      body: JSON.stringify({ sessionId: cfg.sessionId, activeTabId: active?.id || 0, tabs: reports }),
    });
    if (!posted.ok) throw new Error("tabs " + posted.status);
    const inventory = await posted.json();
    const availableTabIds = (inventory.tabs || []).filter((tab) => tab.available).map((tab) => tab.tabId);
    await chrome.storage.local.set({ availableTabIds });
    const commandsResponse = await fetch(
      base + "/v1/extension/commands?sessionId=" + encodeURIComponent(cfg.sessionId),
      { headers: headers(cfg) },
    );
    if (!commandsResponse.ok) throw new Error("commands " + commandsResponse.status);
    const { commands } = await commandsResponse.json();
    let last = active ? "Watching " + (active.title || tabUrl(active)) : "No active tab.";
    for (const command of commands || []) {
      const result = await chrome.tabs.sendMessage(command.tabId, { type: "execute", command });
      await fetch(base + "/v1/extension/result", {
        method: "POST",
        headers: headers(cfg),
        body: JSON.stringify({
          sessionId: cfg.sessionId,
          commandId: command.id,
          ok: Boolean(result?.ok),
          detail: result?.detail || "No detail.",
        }),
      });
      if (!active || command.tabId === active.id) last = result?.detail || last;
    }
    const activeReport = reports.find((tab) => tab.tabId === active?.id);
    await remember({
      connected: Boolean(activeReport),
      detail: activeReport ? last : "The active tab is not an http(s) page.",
      elements: activeReport?.elements?.length || 0,
      activeTabId: active?.id || 0,
    });
  } catch (error) {
    await remember({ connected: false, detail: String(error.message || error) });
  }
}

chrome.runtime.onInstalled.addListener(async () => {
  const stored = await chrome.storage.local.get(null);
  await chrome.storage.local.set({ ...DEFAULTS, ...stored, availableTabIds: stored.availableTabIds ?? null });
  chrome.alarms.create("tick", { periodInMinutes: 0.5 });
  chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});
});

chrome.sidePanel?.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "tick") tick();
});

chrome.tabs.onActivated.addListener(() => tick());
chrome.tabs.onRemoved.addListener(() => tick());
chrome.tabs.onUpdated.addListener((_id, info) => {
  if (info.status === "complete" || info.title || info.url) tick();
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
