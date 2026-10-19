(function (root, factory) {
  const api = factory();
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.LayaController = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  const ELEMENT_CAP = 200;
  const INTERACTIVE = [
    "a[href]",
    "button",
    "input",
    "textarea",
    "select",
    "summary",
    "[role='button']",
    "[role='link']",
    "[role='textbox']",
    "[role='searchbox']",
    "[role='checkbox']",
    "[role='radio']",
    "[role='combobox']",
    "[role='listbox']",
    "[role='option']",
    "[role='menuitem']",
    "[role='menuitemcheckbox']",
    "[role='menuitemradio']",
    "[role='tab']",
    "[role='switch']",
    "[contenteditable='true']",
  ].join(",");

  const TEXT_TYPES = ["text", "email", "search", "tel", "url", "password", "number", ""];

  function accessibleName(el) {
    const labelled = el.getAttribute("aria-label") || el.getAttribute("alt") || el.getAttribute("title");
    if (labelled && labelled.trim()) return labelled.trim().slice(0, 120);
    if (el.labels && el.labels[0]) {
      let fromLabel = "";
      el.labels[0].childNodes.forEach(function (node) {
        if (node.nodeType === 3) fromLabel += node.textContent;
      });
      fromLabel = fromLabel.trim().replace(/\s+/g, " ");
      if (fromLabel) return fromLabel.slice(0, 120);
    }
    const inputType = (el.getAttribute("type") || "").toLowerCase();
    const value = inputType === "password" ? "" : el.value || "";
    const text = el.innerText || el.textContent || el.getAttribute("placeholder") || value || "";
    return String(text).trim().replace(/\s+/g, " ").slice(0, 120);
  }

  function implicitRole(el) {
    const tag = el.tagName;
    if (tag === "A") return "link";
    if (tag === "BUTTON") return "button";
    if (tag === "SELECT") return "combobox";
    if (tag === "TEXTAREA") return "textbox";
    if (tag === "INPUT") {
      const type = (el.getAttribute("type") || "text").toLowerCase();
      if (type === "checkbox" || type === "radio") return type;
      if (type === "submit" || type === "button" || type === "reset" || type === "image") return "button";
      return "textbox";
    }
    return tag.toLowerCase();
  }

  function isEditable(el) {
    if (el.isContentEditable) return true;
    if (el.disabled || el.readOnly) return false;
    if (el.tagName === "TEXTAREA") return true;
    if (el.tagName === "INPUT") {
      return TEXT_TYPES.includes((el.getAttribute("type") || "text").toLowerCase());
    }
    return false;
  }

  function isVisible(el) {
    if (el.hidden || el.getAttribute("aria-hidden") === "true" || el.disabled) return false;
    const type = (el.getAttribute("type") || "").toLowerCase();
    if (type === "hidden") return false;
    const view = el.ownerDocument && el.ownerDocument.defaultView;
    const style = view && view.getComputedStyle ? view.getComputedStyle(el) : null;
    if (style && (style.display === "none" || style.visibility === "hidden")) return false;
    return true;
  }

  function collectElements(doc, limit) {
    const cap = limit || ELEMENT_CAP;
    const view = doc.defaultView || globalThis;
    const out = [];
    doc.querySelectorAll(INTERACTIVE).forEach(function (el) {
      if (out.length >= cap || !isVisible(el)) return;
      if (!el.dataset.layaId) {
        view.__layaSeq = (view.__layaSeq || 0) + 1;
        el.dataset.layaId = "e" + view.__layaSeq;
      }
      const inputType = (el.getAttribute("type") || "").toLowerCase();
      const item = {
        id: el.dataset.layaId,
        role: el.getAttribute("role") || implicitRole(el),
        name: accessibleName(el),
        tag: el.tagName.toLowerCase(),
        editable: isEditable(el),
        inputType: inputType,
        value: inputType === "password" ? "" : String(el.value || "").slice(0, 80),
        href: el.href || "",
      };
      if (el.tagName === "SELECT") {
        item.options = Array.from(el.options || []).map(function (option) {
          return String(option.text || option.value || "").trim();
        }).filter(Boolean).slice(0, 40);
      }
      out.push(item);
    });
    return out;
  }

  function executeAction(doc, command) {
    const type = command && command.type;
    if (type === "navigate") {
      const url = command.url || "";
      if (!/^https?:\/\//i.test(url)) return { ok: false, detail: "Only http(s) URLs can be opened." };
      doc.location.assign(url);
      return { ok: true, detail: "Opening " + url };
    }
    if (type === "scroll" && !command.elementId) {
      const view = doc.defaultView;
      if (view && view.scrollBy) view.scrollBy(0, 480);
      return { ok: true, detail: "Scrolled the page." };
    }
    if (!command.elementId || !/^[A-Za-z0-9_-]+$/.test(command.elementId)) {
      return { ok: false, detail: "Missing element id." };
    }
    const el = doc.querySelector('[data-laya-id="' + command.elementId + '"]');
    if (!el) return { ok: false, detail: "No element " + command.elementId + "." };
    if (el.scrollIntoView) el.scrollIntoView({ block: "center" });
    if (type === "extract") {
      const text = (el.innerText || el.textContent || el.value || "").trim().slice(0, 500);
      return { ok: true, detail: text || "Element is empty." };
    }
    if (type === "click") {
      el.click();
      return { ok: true, detail: "Clicked " + (accessibleName(el) || command.elementId) + "." };
    }
    if (type === "type") {
      if (!isEditable(el)) return { ok: false, detail: "Element is not editable." };
      el.focus();
      el.value = command.text || "";
      const view = doc.defaultView;
      el.dispatchEvent(new view.Event("input", { bubbles: true }));
      el.dispatchEvent(new view.Event("change", { bubbles: true }));
      return { ok: true, detail: "Typed into " + (accessibleName(el) || command.elementId) + "." };
    }
    if (type === "select") {
      if (el.tagName !== "SELECT") return { ok: false, detail: "Element is not a menu." };
      const wanted = String(command.text || "").toLowerCase();
      const option = Array.from(el.options).find(function (item) {
        return item.value.toLowerCase() === wanted || item.text.toLowerCase() === wanted || item.text.toLowerCase().indexOf(wanted) !== -1;
      });
      if (!option) return { ok: false, detail: "Option not found." };
      el.value = option.value;
      el.dispatchEvent(new doc.defaultView.Event("change", { bubbles: true }));
      return { ok: true, detail: "Selected " + option.text + "." };
    }
    if (type === "scroll") return { ok: true, detail: "Scrolled " + (accessibleName(el) || "element") + " into view." };
    return { ok: false, detail: "Unknown action " + type + "." };
  }

  return { collectElements: collectElements, executeAction: executeAction, accessibleName: accessibleName };
});
