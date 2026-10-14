"use strict";
// Actual embedded app.js, synthetic DOM/HTTP only: no backend/browser/network proof.
const fs = require("node:fs");
const vm = require("node:vm");
const assert = require("node:assert/strict");
const { documentFixture, waitFor, tick, script } = require("./test_web_view.js");
const html = fs.readFileSync("internal/web/assets/index.html", "utf8");
const max = 321 * 1048576, high = "9007199254740993";
const hostile = "<script>boom</script>";
const asset = (id = high, restored = false) => ({ id, sha256: (restored ? "b" : "a").repeat(64), format: "png", bytes: 128, width: 2, height: 2, frames: 1, tags: [hostile], emotion: "calm", description: restored ? "new reused ID" : hostile, state: "ready", enabled: false, uses: high, last_used_millis: "0", availability: "unchecked" });
const tests = [];
const test = (name, fn) => tests.push([name, fn]);
async function fixture(config = {}) {
  const doc = documentFixture(), $ = id => doc.getElementById(id), calls = [], urls = [], clicked = [];
  let restored = false, dispatchSnapshot, pendingResolve, readerResolve, textResolve;
  const status = { channel_state: "stopped", channel: {}, summary: {}, logical_attempts: {}, successful_usage: {}, events: [] };
  const file = { size: 4096, name: "<img onerror=synthetic>.bundle", marker: "exact synthetic File" };
  const json = payload => new Response(JSON.stringify(payload), { headers: { "Content-Type": "application/json" } });
  async function fetch(path, options) {
    calls.push({ path, options });
    assert.equal(options.credentials, "same-origin"); assert.equal(options.mode, "same-origin"); assert.equal(options.redirect, "error"); assert.equal(options.cache, "no-store");
    const url = new URL(path, "http://synthetic.invalid");
    if (path === "/api/session") return json({ csrf: "s".repeat(43) });
    if (path === "/api/status") return json(status);
    if (path === "/api/settings") return json({ desired: {}, effective: {}, overrides: {} });
    if (path === "/api/profiles") return json({ custom: [{ id: "sameprofile", name: restored ? "new profile" : "old profile", description: hostile, style: "old style", address: "old address", length: "detailed", sticker: "low", humor: 1 }], builtin_ids: [], active: {} });
    if (url.pathname === "/api/facts") return json(options.method === "GET" ? { items: [{ id: high, content: restored ? "new reused fact" : "old fact", category: "old", importance: 50 }], next_offset: Number(url.searchParams.get("offset")) + 16, has_more: true } : { ok: true });
    if (url.pathname === "/api/history") return json({ items: [{ id: high, content: restored ? "new history" : "old history", reply: hostile }], next_offset: 8, has_more: true });
    if (path === "/api/memory") return json({ candidates: [{ id: high, content: "old candidate", quote: hostile }], derived: { text: "old summary" } });
    if (path === "/api/diagnostics") return json({ status, external_probe: false });
    if (path === "/api/logout") return json({});
    if (url.pathname === "/api/library") {
      if (options.method !== "GET") return json({ ok: true });
      const start = url.searchParams.get("after") === "0" ? BigInt(high) : BigInt(url.searchParams.get("after")) + 1n;
      const items = Array.from({ length: 32 }, (_, i) => asset(String(start + BigInt(i)), restored));
      return json({ items, next_after: items.at(-1).id });
    }
    if (["/api/library/backup", "/api/backup"].includes(path)) {
      if (config.downloadStatus) return new Response("{}", { status: config.downloadStatus });
      if (config.download === "late") return new Promise(resolve => { pendingResolve = resolve; });
      if (config.download === "bodylate") return { ok: true, headers: new Headers(), body: { getReader: () => ({ read: () => new Promise(resolve => { readerResolve = resolve; }), cancel: async () => {}, releaseLock() {} }) } };
      if (config.download === "declared-large") return new Response(new Uint8Array(1), { headers: { "Content-Length": String(max + 1) } });
      if (config.download === "stream-large") return { ok: true, headers: new Headers(), body: { getReader: () => ({ read: async () => ({ done: false, value: { byteLength: max + 1 } }), cancel: async () => { config.cancelled = true; }, releaseLock() {} }) } };
      if (config.download === "truncated") return new Response(new Uint8Array(2), { headers: { "Content-Length": "3" } });
      return new Response(new Uint8Array([1, 2, 3]), { headers: { "Content-Length": "3" } });
    }
    if (["/api/library/restore", "/api/restore"].includes(path)) {
      dispatchSnapshot = snapshot();
      assert.equal(options.body, file, "raw upload keeps the exact captured File");
      assert.equal(options.headers["Content-Type"], "application/octet-stream");
      assert.equal(options.headers["X-MiskoAI-CSRF"], "s".repeat(43));
      if (config.restore === "late") return new Promise(resolve => { pendingResolve = resolve; });
      if (config.restore === "abort") return new Promise((resolve, reject) => options.signal.addEventListener("abort", () => reject(new Error("synthetic abort")), { once: true }));
      if (config.restoreStatus) return new Response("{}", { status: config.restoreStatus });
      restored = true;
      return json(Object.hasOwn(config, "result") ? config.result : { prior_retained: true, channel_paused: true });
    }
    throw new Error("unexpected route " + path);
  }
  const make = doc.createElement;
  doc.createElement = tag => { const element = make(tag); if (tag === "a") element.click = () => { clicked.push({ href: element.href, download: element.download }); if (config.clickError) throw new Error("synthetic click failure"); }; return element; };
  class FixtureURL extends URL {}
  FixtureURL.createObjectURL = blob => { urls.push({ kind: "create", size: blob.size, type: blob.type }); return "blob:synthetic"; };
  FixtureURL.revokeObjectURL = url => urls.push({ kind: "revoke", url });
  class FixtureBlob extends Blob {
    text() {
      const holdRestore = config.restore === "textlate" && dispatchSnapshot;
      const holdRead = config.holdText && calls.at(-1)?.path.startsWith(config.textPath);
      if ((holdRestore || holdRead) && !textResolve) return new Promise(resolve => { textResolve = resolve; });
      return super.text();
    }
  }
  const ctx = vm.createContext({ document: doc, fetch, setTimeout, clearTimeout, Blob: FixtureBlob, AbortController, TextEncoder, URL: FixtureURL, URLSearchParams, console });
  vm.runInContext(script, ctx, { filename: "actual-production-app.js" });
  const idle = () => $("main").getAttribute("aria-busy") === "false";
  await waitFor(idle, "actual production startup");
  assert.equal($("console-view").hidden, false, "session setup succeeded before test");
  async function dispatch(n, type = "click") { assert.ok(n, "required control exists"); n.dispatch(type); await waitFor(idle, "dispatch " + type); }
  const nav = name => doc.querySelectorAll("[data-section]").find(n => n.dataset.section === name);
  const button = (id, label) => $(id).descendants.find(n => n.tagName === "button" && n.textContent === label);
  function snapshot() {
    const values = {};
    for (const id of ["fact-query", "history-query"]) values[id] = $(id).value;
    for (const [id, names] of [["fact-form", ["content", "category"]], ["profile-form", ["id", "name", "description", "style", "address"]], ["library-form", ["tags", "emotion", "description"]]]) for (const name of names) values[id + "/" + name] = $(id).elements.namedItem(name).value;
    for (const id of ["fact-list", "profile-list", "active-profile", "history-list", "candidate-list", "derived-summary", "facts-page", "history-page", "library-list", "library-page", "library-preview-status", "restore-result", "library-bundle-restore-result"]) values[id] = $(id)?.textContent;
    for (const id of ["restore-file", "library-file", "library-bundle-restore-file"]) values[id] = $(id)?.files.length;
    values.preview = $("library-preview").children.length; values.libraryEditor = $("library-editor").hidden;
    values.libraryTitle = $("library-editor-title").textContent; values.factTitle = $("fact-editor-title").textContent; values.profileReadOnly = $("profile-form").elements.namedItem("id").readOnly;
    values.systemVisible = !$("system").hidden; values.busy = $("main").getAttribute("aria-busy");
    return values;
  }
  async function seed() {
    await dispatch(nav("ai")); assert.ok(button("profile-list", "编辑"), "profile seed: " + $("notice").textContent + " routes=" + calls.map(c => c.path).join(",")); await dispatch(button("profile-list", "编辑"));
    await dispatch(nav("memory")); $("fact-query").value = "old-query"; await dispatch($("fact-search"), "submit"); await dispatch($("facts-next"));
    $("history-query").value = "old-history-query"; await dispatch($("history-search"), "submit"); await dispatch($("history-next")); await dispatch(button("fact-list", "编辑 / 更正"));
    await dispatch(nav("library")); await dispatch($("library-next")); assert.ok(button("library-list", "编辑"), "library seed: " + $("notice").textContent); await dispatch(button("library-list", "编辑")); await dispatch(button("library-list", "预览"));
    const img = $("library-preview").children[0]; const late = { load: img.onload, error: img.onerror, img };
    assert.ok($("library-list").textContent.includes(hostile)); assert.equal($("library-list").descendants.filter(n => ["img", "script"].includes(n.tagName)).length, 0);
    $("library-file").files = [{ size: 128 }]; await dispatch(nav("system"));
    $("restore-file").files = [file]; if ($("library-bundle-restore-file")) $("library-bundle-restore-file").files = [file];
    $("restore-result").textContent = "old SQLite result"; if ($("library-bundle-restore-result")) $("library-bundle-restore-result").textContent = "old full result";
    return late;
  }
  async function startRestore(path = "/api/library/restore") {
    const id = path === "/api/restore" ? "restore-form" : "library-bundle-restore-form";
    assert.ok($(id), "full-bundle restore control exists"); $(id).dispatch("submit"); await waitFor(() => $("confirm-dialog").open, "confirmation");
  }
  async function accept(path = "/api/library/restore") { $("confirm-form").dispatch("submit"); await waitFor(() => calls.some(c => c.path === path), "one confirmed dispatch"); }
  return { doc, $, config, file, calls, urls, clicked, idle, dispatch, nav, button, snapshot, seed, startRestore, accept, json, get dispatchSnapshot() { return dispatchSnapshot; }, get textPending() { return !!textResolve; }, get readerPending() { return !!readerResolve; }, resolve: payload => pendingResolve(payload), resolveReader: payload => readerResolve(payload), resolveText: payload => textResolve(JSON.stringify(payload)) };
}
function invalidated(s) {
  for (const [key, value] of Object.entries(s)) {
    if (["libraryEditor", "systemVisible", "busy", "libraryTitle", "factTitle", "profileReadOnly", "preview"].includes(key)) continue;
    assert.equal(value, ["restore-file", "library-file", "library-bundle-restore-file"].includes(key) ? 0 : "", "private reset: " + key);
  }
  assert.equal(s.preview, 0); assert.equal(s.libraryEditor, true); assert.equal(s.libraryTitle, "编辑图片元数据"); assert.equal(s.factTitle, "新增事实"); assert.equal(s.profileReadOnly, false); assert.equal(s.systemVisible, true);
}
test("existing production compiles and starts; new complete controls exist", async () => { const f = await fixture(); for (const id of ["library-bundle-backup", "library-bundle-restore-form", "library-bundle-restore-file", "library-bundle-restore-result"]) assert.ok(f.$(id), "missing complete-bundle control " + id); });
test("visible wording distinguishes complete all-scope bundle and unchanged SQLite limits", async () => { assert.match(html, /321 MiB/); assert.match(html, /256 MiB/); assert.match(html, /所有范围/); assert.match(html, /校验失败/); assert.match(html, /发送历史/); assert.match(html, /旧内容/); });
test("fixed complete download sends empty JSON and revokes its URL immediately", async () => { const f = await fixture(); await f.dispatch(f.$("library-bundle-backup")); const c = f.calls.find(c => c.path === "/api/library/backup"); assert.equal(c.options.method, "POST"); assert.equal(c.options.body, "{}"); assert.equal(c.options.headers["X-MiskoAI-CSRF"], "s".repeat(43)); assert.deepEqual(f.clicked, [{ href: "blob:synthetic", download: "miskoai-library.bundle" }]); assert.deepEqual(f.urls, [{ kind: "create", size: 3, type: "application/octet-stream" }, { kind: "revoke", url: "blob:synthetic" }]); });
test("existing SQLite download retains route filename and empty JSON", async () => { const f = await fixture(); await f.dispatch(f.$("backup")); assert.equal(f.calls.at(-1).path, "/api/backup"); assert.equal(f.calls.at(-1).options.body, "{}"); assert.equal(f.clicked[0].download, "miskoai-backup.db"); });
test("declared and streamed over-limit downloads refuse; partial body refuses", async () => { for (const download of ["declared-large", "stream-large", "truncated"]) { const f = await fixture({ download }); await f.dispatch(f.$("library-bundle-backup")); assert.equal(f.clicked.length, 0); assert.equal(f.urls.length, 0); if (download === "stream-large") assert.equal(f.config.cancelled, true); assert.equal(f.calls.filter(c => c.path === "/api/library/backup").length, 1); } });
test("click failure still removes temporary anchor and revokes URL", async () => { const f = await fixture({ clickError: true }); await f.dispatch(f.$("library-bundle-backup")); assert.equal(f.urls.at(-1).kind, "revoke"); assert.equal(f.doc.body.children.filter(n => n.tagName === "a" && n.download).length, 0); });
test("invalid absent zero unknown fractional negative and excessive File sizes refuse before confirmation/network", async () => { const f = await fixture(); for (const file of [undefined, {}, { size: 0 }, { size: -1 }, { size: 1.5 }, { size: NaN }, { size: Infinity }, { size: "4096" }, { size: max + 1 }]) { f.$("library-bundle-restore-file").files = file ? [file] : []; await f.dispatch(f.$("library-bundle-restore-form"), "submit"); assert.ok(!f.$("confirm-dialog").open); } assert.equal(f.calls.filter(c => c.path === "/api/library/restore").length, 0); });
test("exact maximum File size admitted without copying or encoding", async () => { const f = await fixture(); f.file.size = max; f.$("library-bundle-restore-file").files = [f.file]; await f.startRestore(); await f.accept(); await waitFor(f.idle, "maximum restore"); assert.equal(f.calls.find(c => c.path === "/api/library/restore").options.body, f.file); });
test("confirmation cancel preserves every editor cursor preview result and selected file", async () => { const f = await fixture(); await f.seed(); const before = f.snapshot(); await f.startRestore(); assert.match(f.$("confirm-description").textContent, /校验失败/); assert.equal(f.calls.filter(c => c.path === "/api/library/restore").length, 0); f.$("confirm-cancel").dispatch("click"); await waitFor(f.idle, "cancel"); assert.deepEqual(f.snapshot(), before); assert.equal(f.$("library-bundle-restore-file").files[0], f.file); });
for (const mode of ["success", "failure", "abort", "unauthorized"]) test("confirmed full restore resets BEFORE raw fetch: " + mode, async () => { const f = await fixture(mode === "abort" ? { restore: "abort" } : mode === "failure" ? { restoreStatus: 500 } : mode === "unauthorized" ? { restoreStatus: 401 } : {}); const late = await f.seed(); await f.startRestore(); await f.accept(); invalidated(f.dispatchSnapshot); assert.equal(f.dispatchSnapshot.busy, "true"); if (mode === "abort") f.$("cancel-request").dispatch("click"); await waitFor(f.idle, "restore " + mode); late.load(); late.error(); assert.equal(late.img.onload, null); assert.equal(late.img.onerror, null); assert.equal(f.$("library-preview-status").textContent, ""); assert.equal(f.calls.filter(c => c.path === "/api/library/restore").length, 1); assert.equal(f.calls.filter(c => c.path === "/api/channel/resume").length, 0); assert.equal(f.$("library-bundle-restore-file").files.length, 0); if (mode === "success") { assert.match(f.$("library-bundle-restore-result").textContent, /通道暂停：是/); await f.dispatch(f.nav("library")); assert.ok(f.$("library-list").textContent.includes("new reused ID")); await f.dispatch(f.$("library-form"), "submit"); assert.equal(f.calls.filter(c => c.options.method === "PATCH").length, 0, "old high library ID discarded"); const form = f.$("fact-form"); form.elements.namedItem("content").value = "new explicit fact"; await f.dispatch(form, "submit"); const c = f.calls.filter(c => new URL(c.path, "http://fixture").pathname === "/api/facts" && c.options.method !== "GET").at(-1); assert.equal(c.options.method, "POST"); assert.equal(Object.hasOwn(JSON.parse(c.options.body), "id"), false); } else assert.equal(f.$("library-bundle-restore-result").textContent, ""); });
test("failure keeps caches empty and pagers disabled after busy state ends", async () => { const f = await fixture({ restoreStatus: 500 }); await f.seed(); await f.startRestore(); await f.accept(); await waitFor(f.idle, "failure"); invalidated(f.snapshot()); for (const id of ["facts-prev", "facts-next", "history-prev", "history-next", "library-prev", "library-next"]) assert.equal(f.$(id).disabled, true); });
test("SQLite restore also clears new full-bundle file and result", async () => { const f = await fixture({ restoreStatus: 500 }); await f.seed(); await f.startRestore("/api/restore"); await f.accept("/api/restore"); await waitFor(f.idle, "SQLite failure"); invalidated(f.dispatchSnapshot); assert.equal(f.$("library-bundle-restore-result").textContent, ""); assert.equal(f.$("library-bundle-restore-file").files.length, 0); });
test("logout clears full bundle controls and old preview callbacks", async () => { const f = await fixture(); const late = await f.seed(); await f.dispatch(f.$("logout")); late.load(); late.error(); assert.equal(f.$("library-bundle-restore-file").files.length, 0); assert.equal(f.$("library-bundle-restore-result").textContent, ""); assert.equal(f.$("library-preview-status").textContent, ""); assert.equal(f.$("console-view").hidden, true); });
test("download 401 clears new private controls and produces no artifact", async () => { const f = await fixture({ downloadStatus: 401 }); await f.seed(); await f.dispatch(f.$("library-bundle-backup")); assert.equal(f.$("library-bundle-restore-file").files.length, 0); assert.equal(f.$("library-bundle-restore-result").textContent, ""); assert.equal(f.clicked.length, 0); });
test("late restore reply after logout cannot repopulate complete result or fetch old caches", async () => { const f = await fixture({ restore: "late" }); await f.seed(); await f.startRestore(); await f.accept(); f.$("logout").dispatch("click"); const callCount = f.calls.length; f.resolve(f.json({ prior_retained: true, channel_paused: true })); await waitFor(f.idle, "late session reply"); assert.equal(f.$("console-view").hidden, true); assert.equal(f.$("library-bundle-restore-result").textContent, ""); assert.equal(f.$("library-bundle-restore-file").files.length, 0); assert.equal(f.calls.length, callCount); });
test("late download response after logout cannot create URL", async () => { const f = await fixture({ download: "late" }); f.$("library-bundle-backup").dispatch("click"); await waitFor(() => f.calls.some(c => c.path === "/api/library/backup"), "download pending"); f.$("logout").dispatch("click"); f.resolve(new Response(new Uint8Array(3))); await waitFor(f.idle, "late download"); assert.equal(f.urls.length, 0); assert.equal(f.clicked.length, 0); });
test("logout during JSON text completion cannot populate restore result", async () => { const f = await fixture({ restore: "textlate" }); await f.seed(); await f.startRestore(); await f.accept(); await waitFor(() => f.textPending, "JSON text pending"); f.$("logout").dispatch("click"); const count = f.calls.length; f.resolveText({ prior_retained: true, channel_paused: true }); await waitFor(f.idle, "late text result"); assert.equal(f.$("library-bundle-restore-result").textContent, ""); assert.equal(f.calls.length, count); });
test("SQLite restore late JSON after logout cannot repopulate old result or follow up", async () => {
  const f = await fixture({ restore: "textlate" }); await f.seed(); await f.startRestore("/api/restore"); await f.accept("/api/restore");
  await waitFor(() => f.textPending, "SQLite JSON pending"); f.$("logout").dispatch("click"); const count = f.calls.length;
  f.resolveText({ prior_retained: true, channel_paused: true }); await waitFor(f.idle, "SQLite stale response");
  assert.equal(f.$("restore-result").textContent, ""); assert.equal(f.$("fact-list").textContent, ""); assert.equal(f.calls.length, count);
});
test("private fact GET late JSON after logout cannot repopulate old list or follow up", async () => {
  const f = await fixture(); await f.seed(); f.config.textPath = "/api/facts?"; f.config.holdText = true;
  f.$("refresh-memory").dispatch("click"); await waitFor(() => f.textPending, "facts JSON pending");
  f.$("logout").dispatch("click"); const count = f.calls.length;
  f.resolveText({ items: [{ id: high, content: hostile, category: "old", importance: 50 }], next_offset: 16, has_more: true });
  await waitFor(f.idle, "facts stale response"); assert.equal(f.$("fact-list").textContent, ""); assert.equal(f.$("facts-page").textContent, ""); assert.equal(f.calls.length, count);
});
test("cancel during JSON text completion cannot report restore success or follow up", async () => {
  for (const path of ["/api/library/restore", "/api/restore"]) {
    const f = await fixture({ restore: "textlate" }); await f.seed(); await f.startRestore(path); await f.accept(path);
    await waitFor(() => f.textPending, "cancel JSON pending"); f.$("cancel-request").dispatch("click"); const count = f.calls.length;
    f.resolveText({ prior_retained: true, channel_paused: true }); await waitFor(f.idle, "canceled JSON response");
    invalidated(f.snapshot()); assert.equal(f.calls.length, count); assert.equal(f.calls.filter(c => c.path === path).length, 1);
    assert.ok(!f.$("notice").textContent.includes("请求已完成"));
  }
});
test("session changes while body is pending cannot create URL", async () => { const f = await fixture({ download: "bodylate" }); f.$("library-bundle-backup").dispatch("click"); await tick(); await tick(); f.$("logout").dispatch("click"); f.resolveReader({ done: true }); await waitFor(f.idle, "late body"); assert.equal(f.urls.length, 0); });
test("cancel during final download read cannot create an artifact or report success", async () => {
  for (const id of ["library-bundle-backup", "backup"]) {
    const f = await fixture({ download: "bodylate" }); f.$(id).dispatch("click"); await waitFor(() => f.readerPending, "download reader pending");
    f.$("cancel-request").dispatch("click"); const count = f.calls.length; f.resolveReader({ done: true }); await waitFor(f.idle, "canceled body completion");
    assert.equal(f.urls.length, 0); assert.equal(f.clicked.length, 0); assert.equal(f.calls.length, count); assert.ok(!f.$("notice").textContent.includes("准备下载"));
  }
});
test("nonboolean or missing restore flags refuse success without retry or cache reload", async () => { for (const result of [null, {}, { prior_retained: true }, { prior_retained: "true", channel_paused: true }, { prior_retained: true, channel_paused: 1 }, { prior_retained: false, channel_paused: null }, { prior_retained: hostile, channel_paused: true }]) { const f = await fixture({ result }); await f.seed(); await f.startRestore(); await f.accept(); await waitFor(f.idle, "invalid result"); invalidated(f.snapshot()); assert.equal(f.calls.filter(c => c.path === "/api/library/restore").length, 1); assert.equal(f.calls.filter(c => c.path === "/api/channel/resume").length, 0); assert.ok(!f.$("notice").textContent.includes("请求已完成")); assert.equal(f.doc.descendants.filter(n => n.tagName === "script" && n.textContent.includes("synthetic hostile")).length, 0); } });
test("valid false flags are displayed honestly without claiming automatic resume", async () => { const f = await fixture({ result: { prior_retained: false, channel_paused: false } }); f.$("library-bundle-restore-file").files = [f.file]; await f.startRestore(); await f.accept(); await waitFor(f.idle, "false flags"); assert.match(f.$("library-bundle-restore-result").textContent, /此前快照保留：否/); assert.match(f.$("library-bundle-restore-result").textContent, /通道暂停：否/); assert.equal(f.calls.filter(c => c.path === "/api/channel/resume").length, 0); });
(async () => { let passed = 0, failed = 0; for (const [name, fn] of tests) { try { await fn(); passed++; console.log(JSON.stringify({ test: name, result: "pass" })); } catch (error) { failed++; console.log(JSON.stringify({ test: name, result: "fail", message: error.message, stack: error.stack })); } } console.log(JSON.stringify({ summary: true, passed, failed, browser: false, backend: false, network: false })); process.exitCode = failed ? 1 : 0; })();
