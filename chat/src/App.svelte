<script>
  import { onMount } from "svelte";

  const fallbackApi = `http://${location.hostname}:7505`;
  let apiBase = localStorage.getItem("laya.api") || fallbackApi;
  let sessionId = localStorage.getItem("laya.session") || "local";
  let token = localStorage.getItem("laya.token") || "";
  let selectedTabId = Number(localStorage.getItem("laya.tab") || 0);
  let draft = "";
  let status = null;
  let session = { messages: [], tabs: [] };
  let error = "";
  let sending = false;
  let settingsOpen = false;

  function headers() {
    const result = { "content-type": "application/json" };
    if (token.trim()) result["X-Api-Token"] = token.trim();
    return result;
  }

  function base() {
    return apiBase.replace(/\/$/, "");
  }

  async function refresh() {
    try {
      const statusResponse = await fetch(`${base()}/v1/status`, { headers: headers() });
      const sessionResponse = await fetch(
        `${base()}/v1/sessions/${encodeURIComponent(sessionId || "local")}?tabId=${selectedTabId || 0}`,
        { headers: headers() },
      );
      if (statusResponse.status === 401 || sessionResponse.status === 401) {
        error = "The API rejected the token.";
        return;
      }
      status = await statusResponse.json();
      session = await sessionResponse.json();
      if (!selectedTabId && session.activeTabId) selectedTabId = session.activeTabId;
      error = "";
    } catch (err) {
      error = `Can't reach the API at ${base()}.`;
    }
  }

  function choose(tabId) {
    selectedTabId = tabId;
    localStorage.setItem("laya.tab", String(tabId));
    refresh();
  }

  async function setAvailable(tab, available) {
    const response = await fetch(`${base()}/v1/tabs/availability`, {
      method: "POST",
      headers: headers(),
      body: JSON.stringify({ sessionId: sessionId || "local", tabId: tab.tabId, available }),
    });
    if (!response.ok) {
      error = "Could not update that tab.";
      return;
    }
    if (selectedTabId === tab.tabId) session = await response.json();
    else await refresh();
  }

  function saveSettings() {
    localStorage.setItem("laya.api", apiBase.trim() || fallbackApi);
    localStorage.setItem("laya.session", sessionId.trim() || "local");
    localStorage.setItem("laya.token", token);
    apiBase = apiBase.trim() || fallbackApi;
    sessionId = sessionId.trim() || "local";
    settingsOpen = false;
    refresh();
  }

  async function send(confirmed = false) {
    const message = draft.trim();
    if (!selectedTabId || (!confirmed && !message)) return;
    sending = true;
    try {
      const response = await fetch(`${base()}/v1/chat`, {
        method: "POST",
        headers: headers(),
        body: JSON.stringify({
          sessionId: sessionId || "local",
          tabId: selectedTabId,
          message: confirmed ? "" : message,
          confirmed,
        }),
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || response.statusText);
      if (!confirmed) draft = "";
      session = body.session || session;
      error = "";
    } catch (err) {
      error = err.message;
    } finally {
      sending = false;
    }
  }

  function onKeydown(event) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      send(false);
    }
  }

  $: tabs = session?.tabs || [];
  $: selected = tabs.find((tab) => tab.tabId === selectedTabId) || null;
  $: messages = session?.focusTabId === selectedTabId ? session.messages || [] : session.messages || [];
  $: decision = [...messages].reverse().find((item) => item.role === "assistant");
  $: agent = status?.agent || {};
  $: laya = agent.laya || "offline";
  $: canSend = Boolean(selected?.available);

  onMount(() => {
    refresh();
    const timer = setInterval(refresh, 1500);
    return () => clearInterval(timer);
  });
</script>

<main>
  <aside class="tabs">
    <p class="eyebrow">Open tabs</p>
    <h2>Available to the extension</h2>
    {#if tabs.length === 0}
      <p class="meta">No tabs yet. Open the sidebar on a page so the extension can report them.</p>
    {/if}
    {#each tabs as tab}
      <div class="tab" class:selected={tab.tabId === selectedTabId}>
        <input
          type="checkbox"
          checked={tab.available}
          title="Available to the extension"
          on:change={(event) => setAvailable(tab, event.currentTarget.checked)}
        />
        <button type="button" on:click={() => choose(tab.tabId)}>
          <strong>{tab.title || "Untitled"}</strong>
          <span>{tab.url}</span>
        </button>
        {#if tab.active}<em>active</em>{/if}
      </div>
    {/each}
  </aside>

  <section class="thread">
    <header>
      <div>
        <p class="eyebrow">Command desk</p>
        <h1>{selected?.title || "Laya"}</h1>
      </div>
      <div class="pills">
        <span class:on={status?.api === "ok"}>API</span>
        <span class:on={laya === "ready"}>{laya === "ready" ? "Laya" : laya}</span>
        <button type="button" class="text" on:click={() => (settingsOpen = !settingsOpen)}>Session</button>
      </div>
    </header>

    {#if settingsOpen}
      <form class="settings" on:submit|preventDefault={saveSettings}>
        <label>API <input bind:value={apiBase} spellcheck="false" /></label>
        <label>Session <input bind:value={sessionId} maxlength="64" spellcheck="false" /></label>
        <label>Token <input bind:value={token} type="password" placeholder="optional" /></label>
        <button type="submit">Save</button>
      </form>
    {/if}

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <div class="log">
      {#if !selected}
        <p class="empty">Select a tab the extension has reported. Checked tabs are the ones it may control.</p>
      {:else if messages.length === 0}
        <p class="empty">
          Commands here go to <strong>{selected.title || "this tab"}</strong>.
          The extension sidebar talks to whichever tab is active.
        </p>
      {/if}
      {#each messages as message}
        <article class={message.role}>
          <span>{message.role}</span>
          <p>{message.content}</p>
        </article>
      {/each}
    </div>

    {#if session?.held && canSend}
      <button class="confirm" type="button" disabled={sending} on:click={() => send(true)}>
        Confirm {session.held.type}
      </button>
    {/if}

    <form class="composer" on:submit|preventDefault={() => send(false)}>
      <textarea
        bind:value={draft}
        rows="2"
        placeholder={selected ? "Click Login" : "Select a tab first"}
        disabled={!canSend}
        on:keydown={onKeydown}
      ></textarea>
      <button type="submit" disabled={sending || !canSend || !draft.trim()}>Send</button>
    </form>
    {#if selected && !selected.available}
      <p class="meta">This tab is unchecked, so the extension will not act on it.</p>
    {/if}
    {#if decision?.backend}
      <p class="meta">Last decision via {decision.backend}</p>
    {/if}
  </section>
</main>

<style>
  :global(body) {
    margin: 0;
    background: #14120f;
    color: #f3ecdf;
    font-family: "Segoe UI", sans-serif;
  }
  main {
    display: grid;
    grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
    min-height: 100vh;
  }
  .tabs, .thread { padding: 24px; }
  .tabs { background: #1c1915; border-right: 1px solid #342f28; }
  header, .pills { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
  .eyebrow { margin: 0; letter-spacing: 0.16em; text-transform: uppercase; font-size: 11px; color: #e4b15a; }
  h1 { margin: 0; font-family: Georgia, serif; font-weight: 500; font-size: 36px; }
  h2 { font-family: Georgia, serif; font-weight: 500; font-size: 22px; margin: 6px 0 16px; }
  .tab {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 8px;
    align-items: center;
    margin: 8px 0;
    padding: 8px;
    border-radius: 10px;
  }
  .tab.selected { background: #26221c; }
  .tab button {
    text-align: left;
    background: transparent;
    color: inherit;
    border: 0;
    padding: 0;
    cursor: pointer;
  }
  .tab strong, .tab span { display: block; }
  .tab span { color: #cbbfa8; font-size: 12px; word-break: break-all; }
  .tab em { color: #e4b15a; font-style: normal; font-size: 11px; }
  .pills span, .text {
    border: 1px solid #3a342c;
    border-radius: 999px;
    padding: 4px 10px;
    color: #a39886;
    background: transparent;
    font: inherit;
  }
  .pills span.on { color: #d5ecc4; border-color: #4d6b3c; }
  .log { display: flex; flex-direction: column; gap: 12px; margin: 22px 0; min-height: 40vh; }
  article span { display: block; font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: #a39886; }
  article p { margin: 4px 0 0; white-space: pre-wrap; }
  article.user p { color: #e4b15a; }
  .empty, .meta { color: #cbbfa8; }
  .error { color: #ffb4a8; }
  .settings, .composer { display: grid; gap: 8px; }
  .settings { grid-template-columns: 1fr 160px 160px auto; margin-top: 16px; }
  label { display: grid; gap: 4px; font-size: 12px; color: #cbbfa8; }
  input, textarea {
    background: #26221c;
    color: #f3ecdf;
    border: 1px solid #3a342c;
    border-radius: 8px;
    padding: 8px 10px;
    font: inherit;
  }
  button { font: inherit; }
  .settings button, .composer button, .confirm {
    background: #e4b15a;
    color: #1c1915;
    border: 0;
    border-radius: 8px;
    padding: 10px 14px;
    font-weight: 650;
  }
  .composer { grid-template-columns: 1fr auto; align-items: end; }
  .confirm { margin-bottom: 12px; }
  @media (max-width: 860px) {
    main { grid-template-columns: 1fr; }
    .settings { grid-template-columns: 1fr; }
    .tabs { border-right: 0; border-bottom: 1px solid #342f28; }
  }
</style>
