<script>
  import { onMount } from "svelte";

  const fallbackApi = `http://${location.hostname}:7501`;
  let apiBase = localStorage.getItem("laya.api") || fallbackApi;
  let sessionId = localStorage.getItem("laya.session") || "local";
  let token = localStorage.getItem("laya.token") || "";
  let draft = "";
  let status = null;
  let session = { messages: [] };
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
      const [statusResponse, sessionResponse] = await Promise.all([
        fetch(`${base()}/v1/status`, { headers: headers() }),
        fetch(`${base()}/v1/sessions/${encodeURIComponent(sessionId || "local")}`, { headers: headers() }),
      ]);
      if (statusResponse.status === 401 || sessionResponse.status === 401) {
        error = "The API rejected the token.";
        return;
      }
      status = await statusResponse.json();
      session = await sessionResponse.json();
      error = "";
    } catch (err) {
      error = `Can't reach the API at ${base()}.`;
    }
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
    if (!confirmed && !message) return;
    sending = true;
    try {
      const response = await fetch(`${base()}/v1/chat`, {
        method: "POST",
        headers: headers(),
        body: JSON.stringify({
          sessionId: sessionId || "local",
          message: confirmed ? "" : message,
          confirmed,
        }),
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || response.statusText);
      if (!confirmed) draft = "";
      session = body.session || session;
      await refresh();
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

  $: messages = session?.messages || [];
  $: decision = [...messages].reverse().find((item) => item.role === "assistant");
  $: agent = status?.agent || {};
  $: laya = agent.laya || "offline";
  $: extensionOn = Boolean(session?.extensionConnected);
  $: page = session?.page;

  onMount(() => {
    refresh();
    const timer = setInterval(refresh, 1500);
    return () => clearInterval(timer);
  });
</script>

<main>
  <section class="thread">
    <header>
      <div>
        <p class="eyebrow">Page agent</p>
        <h1>Laya</h1>
      </div>
      <div class="pills">
        <span class:on={status?.api === "ok"}>API</span>
        <span class:on={laya === "ready"}>{laya === "ready" ? "Laya" : laya}</span>
        <span class:on={extensionOn}>Extension</span>
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
      {#if messages.length === 0}
        <p class="empty">
          Ask for a click, a typed value, or a scroll. The extension should report the active tab into session
          <code>{sessionId || "local"}</code>.
          <a href="{base()}/demo" target="_blank" rel="noreferrer">Open the practice page</a>.
        </p>
      {/if}
      {#each messages as message}
        <article class={message.role}>
          <span>{message.role}</span>
          <p>{message.content}</p>
        </article>
      {/each}
    </div>

    {#if session?.held}
      <button class="confirm" type="button" disabled={sending} on:click={() => send(true)}>
        Confirm {session.held.type}{session.held.elementId ? ` ${session.held.elementId}` : ""}
      </button>
    {/if}

    <form class="composer" on:submit|preventDefault={() => send(false)}>
      <textarea
        bind:value={draft}
        rows="2"
        placeholder="Click Login"
        on:keydown={onKeydown}
      ></textarea>
      <button type="submit" disabled={sending || !draft.trim()}>Send</button>
    </form>
  </section>

  <aside>
    <h2>This tab</h2>
    {#if page}
      <p class="title">{page.title || "Untitled"}</p>
      <p class="url">{page.url}</p>
      <p class="meta">{page.elements?.length || 0} controls · seen {page.reportedAt || "—"}</p>
      <ul>
        {#each (page.elements || []).slice(0, 12) as element}
          <li><code>{element.id}</code> {element.role} {element.name}</li>
        {/each}
      </ul>
    {:else}
      <p class="meta">No snapshot yet for this session.</p>
    {/if}

    <h2>Last decision</h2>
    {#if decision}
      <p>{decision.content}</p>
      {#if decision.backend}
        <p class="meta">Backend {decision.backend}</p>
      {/if}
    {:else}
      <p class="meta">Nothing decided yet.</p>
    {/if}
    {#if session?.lastResult}
      <h2>Last result</h2>
      <p>{session.lastResult.detail}</p>
    {/if}
  </aside>
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
    grid-template-columns: minmax(0, 1.6fr) minmax(260px, 0.8fr);
    min-height: 100vh;
  }
  .thread, aside { padding: 28px; }
  aside { background: #1c1915; border-left: 1px solid #342f28; }
  header, .pills { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
  .eyebrow { margin: 0; letter-spacing: 0.16em; text-transform: uppercase; font-size: 11px; color: #e4b15a; }
  h1 { margin: 0; font-family: Georgia, serif; font-weight: 500; font-size: 42px; }
  h2 { font-family: Georgia, serif; font-weight: 500; font-size: 20px; margin: 22px 0 8px; }
  .pills span, .text {
    border: 1px solid #3a342c;
    border-radius: 999px;
    padding: 4px 10px;
    color: #a39886;
    background: transparent;
    font: inherit;
  }
  .pills span.on { color: #d5ecc4; border-color: #4d6b3c; }
  .log { display: flex; flex-direction: column; gap: 12px; margin: 22px 0; min-height: 45vh; }
  article { max-width: 46rem; }
  article span { display: block; font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: #a39886; }
  article p { margin: 4px 0 0; white-space: pre-wrap; }
  article.user p { color: #e4b15a; }
  .empty, .meta, .url { color: #cbbfa8; }
  .url { word-break: break-all; }
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
  ul { padding-left: 18px; }
  li { margin: 4px 0; }
  code { color: #e4b15a; }
  @media (max-width: 860px) {
    main { grid-template-columns: 1fr; }
    .settings { grid-template-columns: 1fr; }
    aside { border-left: 0; border-top: 1px solid #342f28; }
  }
</style>
