"use strict";
// Ephemeral runtime fixture: executes unchanged production app.js in Node VM.
// I2 models intrinsic numeric collection.length and namedItem, matching root's
// observed browser DOM. This deliberately models only surfaces for regression;
// it is not a browser, HTML/CSP renderer, Core/database, or networking test.
const fs = require("node:fs");
const vm = require("node:vm");
const assert = require("node:assert/strict");
const root = process.cwd();
const html = fs.readFileSync(root + "/internal/web/assets/index.html", "utf8");
const script = fs.readFileSync(root + "/internal/web/assets/app.js", "utf8");
const oldFact = { id: "9007199254740993", content: "old fact edit", category: "old", importance: 50, source: "explicit_user", confidence: 1, expires_at: null, updated_at: "2026-10-10T00:00:00Z" };
const oldProfile = { id: "sameprofile", name: "old profile", description: "old description", style: "old style", address: "old address", length: "detailed", sticker: "low", humor: 1 };
const freshFact = { ...oldFact, content: "restored fact with reused ID" };
const freshProfile = { ...oldProfile, name: "restored profile with reused ID" };
class Element {
  constructor(tag, attrs = {}) { this.tagName = tag; this.attributes = { ...attrs }; this.children = []; this.parent = null; this.events = new Map(); this.dataset = {}; this._text = ""; this.value = attrs.value || ""; this.defaultValue = this.value; this.id = attrs.id || ""; this.name = attrs.name || ""; this.className = attrs.class || ""; this.disabled = "disabled" in attrs; this.hidden = "hidden" in attrs; this.readOnly = "readonly" in attrs; this.files = []; this.classList = { toggle: () => {} }; for (const [key, value] of Object.entries(attrs)) if (key.startsWith("data-")) this.dataset[key.slice(5)] = value; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(""); }
  set textContent(value) { this._text = String(value); this.children = []; }
  append(...nodes) { for (const n of nodes) { n.parent = this; this.children.push(n); } }
  replaceChildren(...nodes) { this._text = ""; this.children = []; this.append(...nodes); }
  addEventListener(type, listener) { const list = this.events.get(type) || []; list.push(listener); this.events.set(type, list); }
  dispatch(type) { for (const fn of this.events.get(type) || []) fn({ preventDefault() {} }); }
  setAttribute(key, value) { this.attributes[key] = String(value); }
  removeAttribute(key) { delete this.attributes[key]; }
  getAttribute(key) { return this.attributes[key]; }
  focus() {}
  scrollIntoView() {}
  showModal() { this.open = true; }
  close() { this.open = false; }
  remove() { if (this.parent) this.parent.children = this.parent.children.filter(c => c !== this); }
  get descendants() { return this.children.flatMap(c => [c, ...c.descendants]); }
  get elements() { const list = this.descendants.filter(n => ["input", "textarea", "select", "button"].includes(n.tagName)); const collection = { length: list.length, [Symbol.iterator]: () => list[Symbol.iterator](), namedItem: key => list.find(n => n.name === key || n.id === key) || null }; for (const n of list) if (n.name && !(n.name in collection)) collection[n.name] = n; return collection; }
  reset() { for (const n of this.elements) { n.value = n.defaultValue; if (n.tagName === "input" && n.attributes.type === "file") n.files = []; } }
}
function documentFixture() {
  const doc = new Element("document"); const stack = [doc]; const voidTags = new Set(["meta", "link", "input", "br"]);
  for (const token of html.match(/<!--[\s\S]*?-->|<[^>]+>|[^<]+/g)) {
    if (token.startsWith("<!")) continue;
    if (token.startsWith("</")) { stack.pop(); continue; }
    if (token.startsWith("<")) {
      const tag = /^<([\w-]+)/.exec(token)[1]; const attrs = {};
      for (const match of token.matchAll(/([\w-]+)(?:="([^"]*)")?/g)) if (match.index > 1 + tag.length) attrs[match[1]] = match[2] ?? "";
      const n = new Element(tag, attrs); stack.at(-1).append(n); if (!voidTags.has(tag)) stack.push(n);
    } else stack.at(-1)._text += token;
  }
  for (const n of doc.descendants.filter(n => n.tagName === "select")) { const option = n.children.find(c => "selected" in c.attributes) || n.children[0]; n.value = n.defaultValue = option.attributes.value || ""; }
  const matches = (n, query) => query.startsWith(".") ? n.className.split(" ").includes(query.slice(1)) : query === "[data-section]" ? "section" in n.dataset : n.tagName === query;
  doc.getElementById = id => doc.descendants.find(n => n.id === id);
  doc.createElement = tag => new Element(tag);
  doc.querySelectorAll = query => doc.descendants.filter(n => query.split(",").some(q => matches(n, q)));
  doc.querySelector = query => doc.querySelectorAll(query)[0];
  doc.body = doc.descendants.find(n => n.tagName === "body"); return doc;
}
const tick = () => new Promise(resolve => setImmediate(resolve));
async function waitFor(predicate, label) { for (let i = 0; i < 200; i++) { if (predicate()) return; await tick(); } throw new Error("fixture timeout: " + label); }
function snapshot(doc) {
  const $ = id => doc.getElementById(id);
  return {
    factTitle: $("fact-editor-title").textContent, factContent: $("fact-form").elements.content.value,
    profileID: $("profile-form").elements.id.value, profileReadOnly: $("profile-form").elements.id.readOnly,
    profileValues: Object.fromEntries(["id", "name", "description", "style", "address"].map(key => [key, $("profile-form").elements[key].value])),
    factDisabled: $("fact-form").elements.content.disabled, profileDisabled: $("profile-form").elements.id.disabled,
    factQuery: $("fact-query").value, historyQuery: $("history-query").value,
    facts: $("fact-list").textContent, profiles: $("profile-list").textContent,
    candidates: $("candidate-list").textContent, summary: $("derived-summary").textContent,
    history: $("history-list").textContent, factsPage: $("facts-page").textContent,
    historyPage: $("history-page").textContent, systemVisible: !$("system").hidden,
    busy: $("main").getAttribute("aria-busy"), consoleVisible: !$("console-view").hidden
  };
}
async function fixture(mode) {
  const doc = documentFixture(); const $ = id => doc.getElementById(id); const calls = []; let restored = false; let dispatchSnapshot = null;
  const csrf = "s".repeat(43); const file = { size: 4096, marker: "synthetic-file-object" };
  const status = { channel_state: "unconfigured", heap_bytes: 1048576, uptime_seconds: 60, goroutines: 1, gc_count: 0, has_deepseek_key: false, has_ollama_key: false, channel: { state: "stopped" }, summary: {}, logical_attempts: {}, successful_usage: {}, events: [] };
  const settings = { listen: "127.0.0.1:8787", deepseek_url: "https://api.deepseek.com", model: "deepseek-flash", vision_model: "deepseek-flash", weixin_url: "https://ilinkai.weixin.qq.com", max_output_tokens: 1024, context_bytes: 16384, context_tokens: 4096 };
  async function fetch(path, options) {
    calls.push({ path, method: options.method, body: options.body, csrf: options.headers["X-MiskoAI-CSRF"] }); const url = new URL(path, "http://synthetic.invalid"); let payload;
    if (path === "/api/session") payload = { csrf };
    else if (path === "/api/status") payload = status;
    else if (path === "/api/settings") payload = { desired: settings, effective: settings, overrides: {}, restart_required: false };
    else if (path === "/api/profiles") payload = { custom: [restored ? freshProfile : oldProfile], builtin_ids: ["warm", "concise", "professional"], active: oldProfile };
    else if (url.pathname === "/api/facts") payload = options.method === "GET" ? { items: [restored ? freshFact : oldFact], next_offset: Number(url.searchParams.get("offset")) + 16, has_more: true } : { ok: true };
    else if (url.pathname === "/api/history") payload = { items: [{ id: "original-message", content: restored ? "restored history" : "old history", reply: "text reply", created_at: "2026-10-10T00:00:00Z" }], next_offset: Number(url.searchParams.get("offset")) + 8, has_more: true };
    else if (path === "/api/memory") payload = { candidates: [{ id: "9007199254740994", content: restored ? "restored candidate" : "old candidate", message_id: "original-message", quote: "genuine synthetic quote" }], derived: { text: restored ? "restored summary" : "old summary", watermark: "1", revision: "1" } };
    else if (path === "/api/diagnostics") payload = { product: "MiskoAI", go_version: "synthetic", os: "fixture", architecture: "fixture", external_probe: false, status: { ...status, channel_state: restored ? "restore_paused" : "unconfigured" } };
    else if (path === "/api/restore") {
      dispatchSnapshot = snapshot(doc);
      assert.equal(options.body, file, "restore must preserve locally captured file despite reset");
      assert.equal(options.headers["Content-Type"], "application/octet-stream");
      assert.equal(options.headers["X-MiskoAI-CSRF"], csrf, "restore preserves session CSRF");
      if (mode === "abort") return new Promise((resolve, reject) => options.signal.addEventListener("abort", () => reject(new Error("fixture aborted stream")), { once: true }));
      if (mode === "failure") return new Response(JSON.stringify({ error: "web_operation" }), { status: 500 });
      restored = true; payload = { prior_retained: true, channel_paused: true };
    } else throw new Error("unexpected synthetic route " + path);
    return new Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
  }
  const ctx = vm.createContext({ document: doc, fetch, setTimeout, clearTimeout, Blob, AbortController, TextEncoder, URL, URLSearchParams, console });
  vm.runInContext(script, ctx, { filename: "production-app.js" });
  const idle = () => $("main").getAttribute("aria-busy") === "false";
  await waitFor(idle, "initial session/status");
  async function dispatch(element, event = "click") { element.dispatch(event); await waitFor(idle, "event " + event); }
  const nav = section => doc.querySelectorAll("[data-section]").find(n => n.dataset.section === section);
  await dispatch(nav("ai"));
  const profileRecord = $("profile-list").children.find(n => n.textContent.includes("old profile"));
  if (!profileRecord) throw new Error("fixture profile setup missing: " + JSON.stringify({ notice: $("notice").textContent, login: $("login-message").textContent, calls }));
  await dispatch(profileRecord.descendants.find(n => n.tagName === "button" && n.textContent === "编辑"));
  const profileControls = $("profile-form").elements;
  const populatedProfile = Object.fromEntries(["id", "name", "description", "style", "address", "length", "sticker", "humor"].map(key => [key, profileControls.namedItem(key).value]));
  console.log(JSON.stringify({ trace: "profile_edit", collectionLength: profileControls.length, populated: populatedProfile, readOnly: profileControls.namedItem("id").readOnly, notice: $("notice").textContent }));
  assert.equal(typeof profileControls.length, "number", "real form collection has intrinsic numeric length");
  for (const [key, value] of Object.entries(oldProfile)) assert.equal(String(populatedProfile[key]), String(value), "profile edit populates " + key);
  assert.equal(profileControls.namedItem("id").readOnly, true, "edit locks existing ID after all fields populate");
  await dispatch($("profile-form"), "submit");
  const editedProfileWrite = calls.filter(c => c.path === "/api/profiles" && c.method === "POST").at(-1);
  console.log(JSON.stringify({ trace: "edited_profile_submit", payload: JSON.parse(editedProfileWrite.body) }));
  assert.deepEqual(JSON.parse(editedProfileWrite.body), oldProfile, "actual profile submit preserves every edited field");
  assert.equal($("profile-form").elements.namedItem("id").readOnly, false, "save resets editor ID protection");
  const refreshedRecord = $("profile-list").children.find(n => n.textContent.includes("old profile"));
  await dispatch(refreshedRecord.descendants.find(n => n.tagName === "button" && n.textContent === "编辑"));
  await dispatch(nav("memory"));
  $("fact-query").value = "old-fact-query"; await dispatch($("fact-search"), "submit"); await dispatch($("facts-next"));
  $("history-query").value = "old-history-query"; await dispatch($("history-search"), "submit"); await dispatch($("history-next"));
  await dispatch($("fact-list").descendants.find(n => n.tagName === "button" && n.textContent === "编辑 / 更正"));
  await dispatch(nav("system"));
  $("restore-file").files = [file]; const before = snapshot(doc);
  $("restore-form").dispatch("submit"); await waitFor(() => $("confirm-dialog").open, "restore confirmation");
  assert.deepEqual({ ...snapshot(doc), busy: before.busy, factDisabled: before.factDisabled, profileDisabled: before.profileDisabled }, before, "opening confirmation must preserve old editors/cache");
  assert.equal(calls.filter(c => c.path === "/api/restore").length, 0, "no request before confirmation");
  if (mode === "cancel") { $("confirm-cancel").dispatch("click"); await waitFor(idle, "cancel"); assert.deepEqual(snapshot(doc), before, "pre-dispatch cancel preserves cached edits"); assert.equal($("restore-file").files[0], file); return { doc, calls, before, after: snapshot(doc), dispatchSnapshot }; }
  $("confirm-form").dispatch("submit"); await waitFor(() => dispatchSnapshot !== null, "restore dispatch");
  if (mode === "abort") $("cancel-request").dispatch("click");
  await waitFor(idle, "restore result");
  return { doc, calls, before, after: snapshot(doc), dispatchSnapshot, dispatch, nav };
}
function assertInvalidated(snapshot) {
  assert.equal(snapshot.factTitle, "新增事实"); assert.equal(snapshot.factContent, "");
  assert.equal(snapshot.profileID, ""); assert.equal(snapshot.profileReadOnly, false);
  for (const value of Object.values(snapshot.profileValues)) assert.equal(value, "", "old profile editor field");
  for (const key of ["factQuery", "historyQuery", "facts", "profiles", "candidates", "summary", "history", "factsPage", "historyPage"]) assert.equal(snapshot[key], "", key + " stale database cache");
  assert.equal(snapshot.systemVisible, true); assert.equal(snapshot.consoleVisible, true);
}
(async () => {
  let passed = 0, failed = 0;
  for (const mode of ["success", "failure", "abort", "cancel"]) {
    try {
      const f = await fixture(mode);
      if (mode !== "cancel") {
        console.log(JSON.stringify({ trace: mode, dispatch: f.dispatchSnapshot, after: f.after }));
        if (mode === "success") {
          const doc = f.doc; const form = doc.getElementById("fact-form"); form.elements.content.value = "new explicit fact after restore"; form.elements.category.value = "new";
          await f.dispatch(form, "submit"); const write = f.calls.filter(c => new URL(c.path, "http://fixture").pathname === "/api/facts" && c.method !== "GET").at(-1);
          console.log(JSON.stringify({ trace: "post_restore_fact_write", method: write.method, payload: JSON.parse(write.body) }));
          assert.equal(write.method, "POST", "restore must discard edit target before subsequent submission"); assert.equal(Object.hasOwn(JSON.parse(write.body), "id"), false, "new write cannot target reused restored ID");
          assert.ok(f.after.facts.includes(freshFact.content), "success reloads authoritative facts"); assert.ok(f.after.profiles.includes(freshProfile.name), "success reloads authoritative profiles");
          await f.dispatch(f.nav("memory")); const factRead = f.calls.filter(c => c.path.startsWith("/api/facts?")).at(-1); const query = new URL(factRead.path, "http://fixture").searchParams; assert.equal(query.get("q"), ""); assert.equal(query.get("offset"), "0");
          const profileForm = doc.getElementById("profile-form"); profileForm.elements.id.value = "fresh-profile"; profileForm.elements.name.value = "new explicit profile";
          await f.dispatch(profileForm, "submit"); const profileWrite = f.calls.filter(c => c.path === "/api/profiles" && c.method === "POST").at(-1);
          console.log(JSON.stringify({ trace: "post_restore_profile_write", payload: JSON.parse(profileWrite.body) }));
          assert.equal(JSON.parse(profileWrite.body).id, "fresh-profile"); assert.equal(JSON.parse(profileWrite.body).style, "", "old profile style cannot carry into new write");
        } else assertInvalidated(f.after);
        assertInvalidated(f.dispatchSnapshot); assert.equal(f.dispatchSnapshot.busy, "true", "controls remain pending before restore dispatch");
        assert.equal(f.dispatchSnapshot.factDisabled, true); assert.equal(f.dispatchSnapshot.profileDisabled, true);
        assert.equal(f.after.factContent, ""); assert.equal(f.after.profileID, "");
        assert.equal(f.calls.filter(c => c.path === "/api/restore").length, 1, "no restore retry");
      }
      console.log(JSON.stringify({ test: mode, result: "pass", dispatch: f.dispatchSnapshot, after: f.after })); passed++;
    } catch (error) { console.log(JSON.stringify({ test: mode, result: "fail", message: error.message, stack: error.stack })); failed++; }
  }
  console.log(JSON.stringify({ summary: true, passed, failed, browser: false, network: false })); process.exitCode = failed ? 1 : 0;
})();
