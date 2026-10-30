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
  constructor(tag, attrs = {}) { this.tagName = tag; this.attributes = { ...attrs }; this.children = []; this.parent = null; this.events = new Map(); this.dataset = {}; this._text = ""; this.value = attrs.value || ""; this.defaultValue = this.value; this.checked = this.defaultChecked = "checked" in attrs; this.id = attrs.id || ""; this.name = attrs.name || ""; this.className = attrs.class || ""; this.disabled = "disabled" in attrs; this.hidden = "hidden" in attrs; this.readOnly = "readonly" in attrs; this.files = []; this.classList = { toggle: () => {} }; for (const [key, value] of Object.entries(attrs)) if (key.startsWith("data-")) this.dataset[key.slice(5)] = value; }
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
  reset() { for (const n of this.elements) { n.value = n.defaultValue; n.checked = n.defaultChecked; if (n.tagName === "input" && n.attributes.type === "file") n.files = []; } }
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
module.exports = { Element, documentFixture, waitFor, tick, script };
async function revisionFixture(config = {}) {
  const doc = documentFixture(), $ = id => doc.getElementById(id), calls = [];
  let pendingDefaults;
  const settings = { listen: "127.0.0.1:8787", deepseek_url: "https://api.deepseek.com", model: "saved-model", vision_model: "vision", weixin_url: "https://ilinkai.weixin.qq.com", max_output_tokens: 1024, context_bytes: 16384, context_tokens: 4096 };
  const defaults = { ...settings, model: "default-model" };
  const events = Array.from({ length: 24 }, (_, i) => ({ kind: "event-" + i, at: "2026-10-28T00:00:00Z" }));
  const status = { channel_state: "backoff", diagnostic_code: "transport", heap_bytes: 1048576, uptime_seconds: 7380, goroutines: 2, gc_count: 1, channel: { state: "backoff", received: 3, processed: 2, sent: 1, failed: 1, last_code: "transport", image_decision: "plain_text" }, summary: {}, logical_attempts: {}, successful_usage: {}, events: [...events].reverse() };
  async function fetch(path, options) {
    calls.push({ path, method: options.method, body: options.body });
    const url = new URL(path, "http://fixture"); let payload;
    if (path === "/api/session") { if (config.login) return new Response("{}", { status: 401 }); payload = { csrf: "s".repeat(43) }; }
    else if (path === "/api/bootstrap") payload = { nonce: "n".repeat(43) };
    else if (path === "/api/login") payload = { csrf: "s".repeat(43) };
    else if (path === "/api/status") payload = status;
    else if (path === "/api/settings/defaults") { if (config.holdDefaults) return new Promise(resolve => { pendingDefaults = resolve; }); if (config.defaultsStatus) return new Response("{}", { status: config.defaultsStatus }); payload = defaults; }
    else if (path === "/api/logout") payload = {};
    else if (path === "/api/settings") payload = options.method === "GET" ? { desired: settings, effective: settings, overrides: { model: true } } : { ok: true };
    else if (path === "/api/profiles") payload = options.method === "GET" ? { custom: [oldProfile], active: oldProfile, builtin_ids: ["warm", "concise"] } : { ok: true };
    else if (url.pathname === "/api/events") { const offset = Number(url.searchParams.get("offset")); payload = { items: events.slice(offset, offset + 20), next_offset: Math.min(offset + 20, events.length), has_more: offset === 0 }; }
    else throw new Error("unexpected revision route " + path);
    return new Response(JSON.stringify(payload), { headers: { "Content-Type": "application/json" } });
  }
  class FixedDate extends Date { static now() { return 1793145600000; } }
  vm.runInContext(script, vm.createContext({ document: doc, Date: config.fixedClock ? FixedDate : Date, location: { pathname: config.events ? "/events" : "/" }, crypto: require("node:crypto").webcrypto, fetch, setTimeout, clearTimeout, Blob, AbortController, TextEncoder, URL, URLSearchParams, console }));
  const idle = () => $("main").getAttribute("aria-busy") === "false";
  await waitFor(idle, "revision startup");
  async function dispatch(control, type = "click") { assert.ok(control, "revision control exists"); control.dispatch(type); await waitFor(idle, type); }
  const nav = name => doc.querySelectorAll("[data-section]").find(n => n.dataset.section === name);
  return { doc, $, calls, settings, dispatch, nav, idle, resolveDefaults: () => pendingDefaults(new Response(JSON.stringify(defaults))) };
}
const revisionTests = [
  ["image decision is visible only in diagnostic details and cleared on logout", async () => {
    const f = await revisionFixture();
    const details = f.$("channel-status").descendants.find(n => n.tagName === "details");
    assert.ok(details, "diagnostics are collapsed details");
    assert.ok(details.textContent.includes("配图判定码"));
    assert.ok(details.textContent.includes("plain_text"));
    assert.ok(!f.$("status-cards").textContent.includes("plain_text"));
    await f.dispatch(f.$("logout"));
    assert.ok(!f.$("channel-status").textContent.includes("plain_text"));
  }],
  ["account login posts exactly credentials and nonce", async () => {
    const f = await revisionFixture({ login: true }); assert.ok(f.$("username"), "account field exists");
    f.$("username").value = "operator"; f.$("password").value = "synthetic-password"; await f.dispatch(f.$("login-form"), "submit");
    assert.deepEqual(JSON.parse(f.calls.find(c => c.path === "/api/login").body), { username: "operator", password: "synthetic-password", nonce: "n".repeat(43) }); assert.equal(f.$("password").value, "");
  }],
  ["model edit cancel defaults stay draft until save", async () => {
    const f = await revisionFixture(); await f.dispatch(f.nav("ai")); const input = () => f.$("settings-form").elements.namedItem("model");
    assert.equal(input().readOnly, true, "settings initially read only");
    await f.dispatch(f.$("settings-edit")); assert.equal(input().readOnly, false); input().value = "unsaved";
    await f.dispatch(f.$("settings-cancel")); assert.equal(input().value, "saved-model"); assert.equal(input().readOnly, true);
    await f.dispatch(f.$("settings-edit")); await f.dispatch(f.$("settings-defaults")); assert.equal(input().value, "default-model");
    assert.equal(f.calls.filter(c => c.path === "/api/settings" && c.method === "POST").length, 0);
    assert.ok(f.$("settings-fields").textContent.includes("ENV")); await f.dispatch(f.$("settings-form"), "submit");
    assert.equal(JSON.parse(f.calls.find(c => c.path === "/api/settings" && c.method === "POST").body).model, "default-model"); assert.equal(input().readOnly, true);
  }],
  ["dashboard compact uptime latest three and dedicated event paging", async () => {
    const f = await revisionFixture(); assert.ok(f.$("status-cards").textContent.includes("2 小时 3 分钟"));
    assert.equal(f.$("events").children.length, 3); assert.ok(f.$("events").children[0].textContent.includes("event-0"));
    assert.equal(f.$("events-more").getAttribute("href"), "/events"); assert.ok(f.$("channel-status").textContent.includes("transport"));
    const e = await revisionFixture({ events: true, login: true }); e.$("username").value = "admin"; e.$("password").value = "synthetic-password"; await e.dispatch(e.$("login-form"), "submit");
    assert.equal(e.$("event-page").hidden, false); assert.equal(e.$("event-list").children.length, 20); assert.equal(e.$("events-back").getAttribute("href"), "/");
    await e.dispatch(e.$("events-next")); assert.equal(e.$("event-list").children.length, 4); assert.ok(e.calls.some(c => c.path === "/api/events?offset=20&limit=20"));
    await e.dispatch(e.$("events-prev")); assert.equal(e.$("event-list").children.length, 20);
  }],
  ["personas hide builtins generate IDs preserve metadata and byte limits", async () => {
    const f = await revisionFixture(); await f.dispatch(f.nav("ai")); assert.ok(!f.$("profile-list").textContent.includes("温暖自然"));
    assert.equal(f.$("profile-form").elements.namedItem("id").getAttribute("type"), "hidden");
    const record = f.$("profile-list").children[0]; await f.dispatch(record.descendants.find(n => n.tagName === "button" && n.textContent === "编辑"));
    f.$("profile-form").elements.namedItem("name").value = "renamed"; await f.dispatch(f.$("profile-form"), "submit");
    assert.deepEqual(JSON.parse(f.calls.find(c => c.path === "/api/profiles" && c.method === "POST").body), { ...oldProfile, name: "renamed" });
    await f.dispatch(f.$("new-profile")); const form = f.$("profile-form"); form.elements.namedItem("name").value = "new persona"; form.elements.namedItem("style").value = "中".repeat(171);
    await f.dispatch(form, "submit"); assert.equal(f.calls.filter(c => c.path === "/api/profiles" && c.method === "POST").length, 1, "513 byte style refused");
    form.elements.namedItem("style").value = "中".repeat(170) + "ab"; await f.dispatch(form, "submit");
    const body = JSON.parse(f.calls.filter(c => c.path === "/api/profiles" && c.method === "POST").at(-1).body); assert.match(body.id, /^[a-z0-9_-]{1,64}$/); assert.notEqual(body.id, oldProfile.id); assert.equal(body.style, "中".repeat(170) + "ab");
  }],
  ["default draft late responses cannot survive logout or cancellation", async () => {
    for (const control of ["logout", "cancel-request"]) {
      const f = await revisionFixture({ holdDefaults: true }); await f.dispatch(f.nav("ai")); await f.dispatch(f.$("settings-edit"));
      f.$("settings-form").elements.namedItem("model").value = "private-draft"; f.$("settings-defaults").dispatch("click");
      await waitFor(() => f.calls.some(c => c.path === "/api/settings/defaults"), "pending default draft"); f.$(control).dispatch("click"); f.resolveDefaults(); await waitFor(f.idle, "late defaults");
      assert.equal(f.calls.filter(c => c.path === "/api/settings" && c.method === "POST").length, 0);
      if (control === "logout") { assert.equal(f.$("settings-fields").textContent, ""); assert.equal(f.$("console-view").hidden, true); }
      else { assert.equal(f.$("settings-form").elements.namedItem("model").value, "private-draft"); assert.ok(!f.$("notice").textContent.includes("已填入默认值")); }
    }
  }],
  ["defaults session expiry clears model and persona data without retry", async () => {
    const f = await revisionFixture({ defaultsStatus: 401 }); await f.dispatch(f.nav("ai")); await f.dispatch(f.$("settings-edit")); await f.dispatch(f.$("settings-defaults"));
    assert.equal(f.$("console-view").hidden, true); assert.equal(f.$("settings-fields").textContent, ""); assert.equal(f.$("profile-list").textContent, ""); assert.equal(f.calls.filter(c => c.path === "/api/settings/defaults").length, 1);
  }],
  ["new persona IDs differ across tabs at the same clock time", async () => {
    const ids = [];
    for (let i = 0; i < 2; i++) { const f = await revisionFixture({ fixedClock: true }); await f.dispatch(f.nav("ai")); f.$("profile-form").elements.namedItem("name").value = "new persona"; await f.dispatch(f.$("profile-form"), "submit"); ids.push(JSON.parse(f.calls.find(c => c.path === "/api/profiles" && c.method === "POST").body).id); }
    assert.notEqual(ids[0], ids[1], "separate tabs cannot overwrite a new persona through identical generated IDs");
  }]
];
if (require.main === module) (async () => {
  let passed = 0, failed = 0;
  for (const [name, fn] of revisionTests) { try { await fn(); console.log(JSON.stringify({ test: name, result: "pass" })); passed++; } catch (error) { console.log(JSON.stringify({ test: name, result: "fail", message: error.message, stack: error.stack })); failed++; } }
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
          assert.match(JSON.parse(profileWrite.body).id, /^persona_[a-z0-9_]+$/, "new profile ID is generated internally"); assert.notEqual(JSON.parse(profileWrite.body).id, oldProfile.id, "restore discards previous profile edit target"); assert.equal(JSON.parse(profileWrite.body).style, "", "old profile style cannot carry into new write");
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
