const DEFAULTS = {
  apiBase: "http://127.0.0.1:7505",
  sessionId: "local",
  token: "",
  enabled: true,
  availableTabIds: null,
};

const ports = new Set();
let socket = null;
let socketKey = "";
let retryMs = 1000;
let retryTimer = 0;
let publishTimer = 0;
let publishing = false;
let publishAgain = false;
let latestSession = null;

async function config() {
  const stored = await chrome.storage.local.get(DEFAULTS);
  return { ...DEFAULTS, ...stored };
}

function tabUrl(tab) {
  return tab?.url || tab?.pendingUrl || "";
}

function socketURL(cfg) {
  const base = cfg.apiBase.replace(/\/$/, "").replace(/^http/, "ws");
  const query = new URLSearchParams({ sessionId: cfg.sessionId || "local", role: "extension" });
  if (cfg.token) query.set("token", cfg.token);
  return base + "/v1/ws?" + query.toString();
}

function postSession(session) {
  latestSession = session;
  for (const port of ports) {
    port.postMessage({ type: "session", session });
  }
}

async function remember(patch) {
  const current = await chrome.storage.local.get({ status: {} });
  await chrome.storage.local.set({ status: { ...current.status, ...patch, at: new Date().toISOString() } });
}

async function collectTabs() {
  const cfg = await config();
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
  return { cfg, active, reports };
}

function send(message) {
  if (socket?.readyState !== WebSocket.OPEN) return false;
  socket.send(JSON.stringify(message));
  return true;
}

async function publishTabs() {
  const { cfg, active, reports } = await collectTabs();
  if (!cfg.enabled) return;
  const sent = send({ type: "tabs", activeTabId: active?.id || 0, tabs: reports });
  const activeReport = reports.find((tab) => tab.tabId === active?.id);
  await remember({
    connected: sent && Boolean(activeReport),
    detail: sent
      ? activeReport
        ? "Watching " + (active.title || tabUrl(active))
        : "The active tab is not an http(s) page."
      : "Reconnecting to the gateway.",
    elements: activeReport?.elements?.length || 0,
    activeTabId: active?.id || 0,
  });
}

function schedulePublish() {
  clearTimeout(publishTimer);
  publishTimer = setTimeout(runPublish, 300);
}

async function runPublish() {
  if (publishing) {
    publishAgain = true;
    return;
  }
  publishing = true;
  try {
    await publishTabs();
  } finally {
    publishing = false;
    if (publishAgain) {
      publishAgain = false;
      schedulePublish();
    }
  }
}

async function executeCommands(commands) {
  const [active] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  for (const command of commands || []) {
    let result = { ok: false, detail: "The tab did not accept the command." };
    try {
      result = (await chrome.tabs.sendMessage(command.tabId, { type: "execute", command })) || result;
    } catch (error) {
      result = { ok: false, detail: String(error.message || error) };
    }
    send({
      type: "result",
      commandId: command.id,
      ok: Boolean(result.ok),
      detail: result.detail || "No detail.",
    });
    if (!active || command.tabId === active.id) {
      await remember({ connected: true, detail: result.detail || "Ran " + command.type + "." });
    }
  }
}

async function ensureSocket() {
  const cfg = await config();
  if (!cfg.enabled) return;
  const nextKey = socketURL(cfg);
  if (socket && socketKey === nextKey && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
    return;
  }
  socketKey = nextKey;
  if (socket) socket.close();
  const opened = new WebSocket(nextKey);
  socket = opened;
  opened.onopen = () => {
    retryMs = 1000;
    schedulePublish();
  };
  opened.onmessage = (event) => {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch {
      return;
    }
    if (msg.type === "session") {
      postSession(msg.session);
      const availableTabIds = (msg.session?.tabs || []).filter((tab) => tab.available).map((tab) => tab.tabId);
      chrome.storage.local.set({ availableTabIds });
    }
    if (msg.type === "commands") executeCommands(msg.commands);
  };
  opened.onclose = () => {
    if (socket !== opened) return;
    socket = null;
    clearTimeout(retryTimer);
    retryTimer = setTimeout(ensureSocket, retryMs);
    retryMs = Math.min(retryMs * 2, 15000);
  };
}

chrome.runtime.onInstalled.addListener(async () => {
  const stored = await chrome.storage.local.get(null);
  await chrome.storage.local.set({ ...DEFAULTS, ...stored, availableTabIds: stored.availableTabIds ?? null });
  chrome.alarms.clear("tick");
  chrome.alarms.create("socket", { periodInMinutes: 1 });
  chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});
});

chrome.sidePanel?.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});
chrome.alarms.clear("tick");
chrome.alarms.create("socket", { periodInMinutes: 1 });

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "socket" || alarm.name === "tick") ensureSocket();
});

chrome.tabs.onActivated.addListener(() => schedulePublish());
chrome.tabs.onRemoved.addListener(() => schedulePublish());
chrome.tabs.onUpdated.addListener((_id, info) => {
  if (info.status === "complete" || info.url) schedulePublish();
});

chrome.runtime.onConnect.addListener((port) => {
  if (port.name !== "sidebar") return;
  ports.add(port);
  port.onDisconnect.addListener(() => ports.delete(port));
  if (latestSession) port.postMessage({ type: "session", session: latestSession });
  port.onMessage.addListener((message) => {
    if (message?.type === "watch") send({ type: "watch", tabId: message.tabId || 0 });
    if (message?.type === "sync") schedulePublish();
  });
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === "page") {
    schedulePublish();
    sendResponse({ ok: true });
    return false;
  }
  if (message?.type === "tick") {
    socketKey = "";
    ensureSocket().then(() => sendResponse({ ok: true }));
    return true;
  }
  return false;
});

ensureSocket();
