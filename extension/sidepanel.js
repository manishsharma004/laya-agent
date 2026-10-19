const title = document.getElementById("title");
const pill = document.getElementById("pill");
const urlLine = document.getElementById("url");
const banner = document.getElementById("banner");
const log = document.getElementById("log");
const confirmButton = document.getElementById("confirm");
const draft = document.getElementById("draft");
const api = document.getElementById("api");
const session = document.getElementById("session");
const token = document.getElementById("token");
const settings = document.getElementById("settings");

const DEFAULTS = { apiBase: "http://127.0.0.1:7505", sessionId: "local", token: "", enabled: true };

async function cfg() {
  return { ...DEFAULTS, ...(await chrome.storage.local.get(DEFAULTS)) };
}

function headers(config) {
  const result = { "content-type": "application/json" };
  if (config.token) result["X-Api-Token"] = config.token;
  return result;
}

async function activeTab() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab;
}

let renderedKey = "";

function viewKey(sessionView, tab) {
  const focus = (sessionView.tabs || []).find((item) => item.tabId === tab?.id);
  const messages = (sessionView.messages || [])
    .map((message) => [message.role, message.at, message.content].join("\u0000"))
    .join("\n");
  return JSON.stringify({
    tabId: tab?.id || 0,
    title: focus?.title || tab?.title || "",
    url: focus?.url || "",
    available: !focus || focus.available,
    connected: Boolean(sessionView.extensionConnected),
    held: sessionView.held?.type || "",
    messages,
  });
}

function render(sessionView, tab) {
  const key = viewKey(sessionView, tab);
  if (key === renderedKey) return;
  renderedKey = key;
  const focus = (sessionView.tabs || []).find((item) => item.tabId === tab?.id);
  title.textContent = focus?.title || tab?.title || "No tab";
  urlLine.textContent = focus?.url || "";
  const available = !focus || focus.available;
  pill.textContent = available ? "live" : "off";
  pill.classList.toggle("on", available && sessionView.extensionConnected);
  const stick = log.scrollHeight - log.scrollTop - log.clientHeight < 24;
  log.replaceChildren();
  for (const message of sessionView.messages || []) {
    const article = document.createElement("article");
    article.className = message.role;
    const who = document.createElement("span");
    who.textContent = message.role;
    const body = document.createElement("p");
    body.textContent = message.content;
    article.append(who, body);
    log.append(article);
  }
  if (stick) log.scrollTop = log.scrollHeight;
  if (sessionView.held && available) {
    confirmButton.hidden = false;
    confirmButton.textContent = "Confirm " + sessionView.held.type;
  } else {
    confirmButton.hidden = true;
  }
  if (!available) {
    banner.hidden = false;
    banner.textContent = "This tab is turned off in the command desk.";
  } else if (banner.textContent === "This tab is turned off in the command desk.") {
    banner.hidden = true;
    banner.textContent = "";
  }
}

async function loadSettings() {
  const config = await cfg();
  api.value = config.apiBase;
  session.value = config.sessionId;
  token.value = config.token;
}

let sidebarPort = null;

function watchTab(tab) {
  if (sidebarPort && tab?.id) sidebarPort.postMessage({ type: "watch", tabId: tab.id });
}

function connectSidebar() {
  if (sidebarPort) return;
  const port = chrome.runtime.connect({ name: "sidebar" });
  sidebarPort = port;
  port.onMessage.addListener(async (message) => {
    if (message?.type !== "session") return;
    const tab = await activeTab();
    const focus = message.session?.focusTabId;
    if (focus && tab?.id && focus !== tab.id) return;
    render(message.session, tab);
    if (banner.textContent.startsWith("session ")) {
      banner.hidden = true;
      banner.textContent = "";
    }
  });
  port.onDisconnect.addListener(() => {
    if (sidebarPort === port) sidebarPort = null;
    setTimeout(connectSidebar, 300);
  });
  activeTab().then(watchTab);
}

async function send(confirmed) {
  const config = await cfg();
  const tab = await activeTab();
  const message = draft.value.trim();
  if (!tab?.id || (!confirmed && !message)) return;
  const response = await fetch(config.apiBase.replace(/\/$/, "") + "/v1/chat", {
    method: "POST",
    headers: headers(config),
    body: JSON.stringify({
      sessionId: config.sessionId,
      tabId: tab.id,
      message: confirmed ? "" : message,
      confirmed,
    }),
  });
  const body = await response.json();
  if (!response.ok) {
    banner.hidden = false;
    banner.textContent = body.error || "The agent did not accept that.";
    return;
  }
  if (!confirmed) draft.value = "";
  render(body.session, tab);
}

document.getElementById("composer").addEventListener("submit", (event) => {
  event.preventDefault();
  send(false);
});
confirmButton.addEventListener("click", () => send(true));
document.getElementById("settings-toggle").addEventListener("click", () => {
  settings.hidden = !settings.hidden;
});
settings.addEventListener("submit", async (event) => {
  event.preventDefault();
  await chrome.storage.local.set({
    apiBase: api.value.trim() || DEFAULTS.apiBase,
    sessionId: session.value.trim() || "local",
    token: token.value,
  });
  chrome.runtime.sendMessage({ type: "tick" });
  banner.hidden = false;
  banner.textContent = "Saved.";
  const previous = sidebarPort;
  sidebarPort = null;
  previous?.disconnect();
  connectSidebar();
});
chrome.tabs.onActivated.addListener(() => activeTab().then(watchTab));
loadSettings();
connectSidebar();
