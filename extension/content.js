chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message?.type === "snapshot") {
    sendResponse({
      url: location.href,
      title: document.title,
      elements: globalThis.LayaController.collectElements(document),
    });
    return false;
  }
  if (message?.type === "execute") {
    sendResponse(globalThis.LayaController.executeAction(document, message.command));
    return false;
  }
  return false;
});
