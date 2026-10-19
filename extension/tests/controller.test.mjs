import assert from "node:assert/strict";
import test from "node:test";
import { JSDOM } from "jsdom";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { collectElements, executeAction } = require("../lib/controller.js");

function dom(html) {
  return new JSDOM(`<!doctype html><title>Practice</title><body>${html}</body>`, { url: "https://example.com/practice" });
}

test("collects visible controls and hides passwords", () => {
  const { window } = dom(`
    <button>Login</button>
    <input type="search" placeholder="Search" value="old" />
    <input type="password" value="secret" />
    <input type="hidden" value="nope" />
    <a href="https://example.com/more">More info</a>
  `);
  const elements = collectElements(window.document);
  assert.equal(elements.map((item) => item.name).join(","), "Login,Search,,More info");
  assert.equal(elements[2].value, "");
  assert.equal(elements[1].editable, true);
  assert.equal(elements[0].id, "e1");
});

test("clicks, types, and selects", () => {
  const { window } = dom(`
    <button id="login">Login</button>
    <input type="search" placeholder="Search" />
    <select><option>Red</option><option>Blue</option></select>
  `);
  const elements = collectElements(window.document);
  const clicked = [];
  window.document.querySelector("button").click = () => clicked.push("login");
  const click = executeAction(window.document, { type: "click", elementId: elements[0].id });
  assert.equal(click.ok, true);
  assert.deepEqual(clicked, ["login"]);
  const typed = executeAction(window.document, { type: "type", elementId: elements[1].id, text: "hello" });
  assert.equal(typed.ok, true);
  assert.equal(window.document.querySelector("input").value, "hello");
  const selected = executeAction(window.document, { type: "select", elementId: elements[2].id, text: "Blue" });
  assert.equal(selected.ok, true);
  assert.equal(window.document.querySelector("select").value, "Blue");
});

test("refuses non-http navigation", () => {
  const { window } = dom("<p>Hi</p>");
  const result = executeAction(window.document, { type: "navigate", url: "javascript:alert(1)" });
  assert.equal(result.ok, false);
});
