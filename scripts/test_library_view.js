"use strict";
// Executes actual production JavaScript with synthetic DOM/HTTP boundaries.
// This is development-only evidence, not browser/CSP/backend/file-picker evidence.
const vm = require("node:vm");
const assert = require("node:assert/strict");
const { documentFixture, waitFor, script } = require("./test_web_view.js");
const high = "9007199254740993";
const asset = (id = high) => ({ id, sha256: "a".repeat(64), format: "gif", bytes: 128, width: 2, height: 3, frames: 2, tags: ["<img src=x onerror=boom>"], emotion: "calm", description: "<script>boom</script>", state: "ready", enabled: false, uses: "9007199254740994", last_used_millis: "253402300799999", availability: "unchecked" });
async function fixture(config = {}) {
  const doc = documentFixture(), $ = id => doc.getElementById(id), calls = [];
  const status = { channel_state: "unconfigured", heap_bytes: 0, uptime_seconds: 0, goroutines: 1, gc_count: 0, channel: {}, summary: {}, logical_attempts: {}, successful_usage: {}, events: [] };
  let dispatchSnapshot, restored = false;
  async function fetch(path, options) {
    const url = new URL(path, "http://synthetic.invalid");
    calls.push({ path, method: options.method, body: options.body, options });
    assert.equal(options.credentials, "same-origin"); assert.equal(options.mode, "same-origin"); assert.equal(options.redirect, "error");
    let payload;
    if (path === "/api/session") payload = { csrf: "s".repeat(43) };
    else if (path === "/api/status") payload = status;
    else if (path === "/api/settings") payload = { desired: {}, effective: {}, overrides: {} };
    else if (path === "/api/profiles") payload = { custom: [], builtin_ids: [], active: {} };
    else if (url.pathname === "/api/facts") payload = { items: [], next_offset: 0, has_more: false };
    else if (url.pathname === "/api/history") payload = { items: [], next_offset: 0, has_more: false };
    else if (path === "/api/memory") payload = { candidates: [], derived: { text: "" } };
    else if (path === "/api/diagnostics") payload = { status, external_probe: false };
    else if (path === "/api/logout") payload = {};
    else if (path === "/api/restore") {
      dispatchSnapshot = privateSnapshot();
      if (config.restore === "abort") return new Promise((resolve, reject) => options.signal.addEventListener("abort", () => reject(new Error("synthetic abort")), { once: true }));
      if (config.restore === "failure") return new Response("{}", { status: 500 });
      restored = true; payload = { prior_retained: true, channel_paused: true };
    } else if (url.pathname === "/api/library") {
      assert.ok([...url.searchParams.keys()].every(k => ["after", "limit", "id"].includes(k)), "no scope/path/usage query");
      if (options.method === "GET") {
        assert.equal(url.searchParams.get("limit"), "32");
        if (config.readStatus) return new Response("{}", { status: config.readStatus });
        const after = url.searchParams.get("after");
        const items = config.pages ? config.pages(after) : [asset()];
        payload = { items, next_after: config.nextAfter ?? (items.at(-1)?.id || after) };
      } else {
        assert.equal(options.headers["X-MiskoAI-CSRF"], "s".repeat(43));
        if (config.write === "failure") return new Response("{}", { status: 409 });
        if (config.write === "abort") return new Promise((resolve, reject) => options.signal.addEventListener("abort", () => reject(new Error("synthetic abort")), { once: true }));
        if (options.method === "POST") { assert.equal(options.headers["Content-Type"], "application/octet-stream"); payload = { item: asset(), created: config.created !== false }; }
        else if (options.method === "PATCH") {
          assert.deepEqual(Object.keys(JSON.parse(options.body)).sort(), ["description", "emotion", "enabled", "id", "tags"]);
          payload = { ok: true };
        } else if (options.method === "DELETE") { assert.equal(url.searchParams.get("id"), high); payload = { ok: true }; }
        else throw new Error("foreign method");
      }
    } else throw new Error("unexpected route " + path);
    return new Response(JSON.stringify(payload), { headers: { "Content-Type": "application/json" } });
  }
  vm.runInContext(script, vm.createContext({ document: doc, fetch, setTimeout, clearTimeout, Blob, AbortController, TextEncoder, URL, URLSearchParams, console }), { filename: "production-app.js" });
  const idle = () => $("main").getAttribute("aria-busy") === "false";
  await waitFor(idle, "startup");
  async function dispatch(n, type = "click") { assert.ok(n, "control exists"); n.dispatch(type); await waitFor(idle, "event " + type); }
  const nav = name => doc.querySelectorAll("[data-section]").find(n => n.dataset.section === name);
  const button = label => $("library-list").descendants.find(n => n.tagName === "button" && n.textContent === label);
  function privateSnapshot() { return { rows: $("library-list")?.textContent, page: $("library-page")?.textContent, tags: $("library-form")?.elements.namedItem("tags")?.value, editor: $("library-editor")?.hidden, files: $("library-file")?.files.length, preview: $("library-preview")?.children.length, previewStatus: $("library-preview-status")?.textContent }; }
  return { doc, $, calls, nav, button, dispatch, idle, privateSnapshot, config, get dispatchSnapshot() { return dispatchSnapshot; }, get restored() { return restored; } };
}
const tests = [];
function test(name, fn) { tests.push([name, fn]); }
async function library(config) { const f = await fixture(config); assert.ok(f.nav("library"), "usable library navigation exists"); await f.dispatch(f.nav("library")); return f; }
test("high IDs and hostile metadata stay exact inert text", async () => {
  const f = await library(); const text = f.$("library-list").textContent;
  for (const v of [high, asset().uses, asset().last_used_millis, asset().description, asset().tags[0]]) assert.ok(text.includes(v), v);
  assert.ok(text.includes("未校验")); assert.equal(f.$("library-list").descendants.filter(n => ["img", "script"].includes(n.tagName)).length, 0);
  await f.dispatch(f.button("编辑")); assert.equal(f.$("library-form").elements.namedItem("description").value, asset().description);
  await f.dispatch(f.$("library-form"), "submit"); const write = f.calls.find(c => c.method === "PATCH");
  assert.deepEqual(JSON.parse(write.body), { id: high, tags: asset().tags, emotion: "calm", description: asset().description, enabled: false });
});
test("raw upload and duplicate are one write, no automatic metadata or enable", async () => {
  for (const created of [true, false]) { const f = await library({ created }); const file = { size: 128, marker: "synthetic GIF" }; f.$("library-file").files = [file]; await f.dispatch(f.$("library-upload-form"), "submit");
    const writes = f.calls.filter(c => c.path === "/api/library" && c.method !== "GET"); assert.equal(writes.length, 1); assert.equal(writes[0].method, "POST"); assert.equal(writes[0].body, file);
    assert.equal(f.$("library-file").files.length, 0); assert.ok(f.$("notice").textContent.includes(created ? "禁用" : "保留"));
    assert.equal(new URL(f.calls.filter(c => c.method === "GET" && c.path.startsWith("/api/library?")).at(-1).path, "http://fixture").searchParams.get("after"), "0");
  }
});
test("invalid upload refuses before fetch", async () => {
  const f = await library(); for (const size of [0, 4194305]) { f.$("library-file").files = [{ size }]; await f.dispatch(f.$("library-upload-form"), "submit"); } assert.equal(f.calls.filter(c => c.method === "POST").length, 0);
});
test("metadata byte/control/count bounds and boolean enabled", async () => {
  const f = await library(); await f.dispatch(f.button("编辑")); const form = f.$("library-form"), fields = form.elements;
  const invalid = [["tags", "中".repeat(11)], ["tags", "same\nsame"], ["tags", Array.from({ length: 17 }, (_, i) => "t" + i).join("\n")], ["tags", "a\nb\n"], ["tags", "x\u007f"], ["emotion", "中".repeat(22)], ["emotion", "x\n"], ["description", "中".repeat(342)], ["description", "x\0"]];
  for (const [key, value] of invalid) { await f.dispatch(f.button("编辑")); fields.namedItem(key).value = value; await f.dispatch(form, "submit"); }
  assert.equal(f.calls.filter(c => c.method === "PATCH").length, 0);
  await f.dispatch(f.button("编辑")); fields.namedItem("tags").value = "中".repeat(10) + "ab"; fields.namedItem("emotion").value = "e".repeat(64); fields.namedItem("description").value = "d".repeat(1024); fields.namedItem("enabled").checked = true;
  await f.dispatch(form, "submit"); const body = JSON.parse(f.calls.find(c => c.method === "PATCH").body); assert.equal(body.enabled, true); assert.equal(body.description.length, 1024);
});
test("four bounded string cursor pages and previous navigation", async () => {
  const pages = after => { const first = BigInt(after) + (after === "0" ? BigInt(high) : 1n); return Array.from({ length: 32 }, (_, i) => asset(String(first + BigInt(i)))); };
  const f = await library({ pages }); for (let i = 0; i < 3; i++) await f.dispatch(f.$("library-next"));
  assert.equal(f.$("library-next").disabled, true); const last = f.calls.at(-1).path; assert.ok(last.includes("after=9007199254741088"));
  await f.dispatch(f.$("library-next")); assert.equal(f.calls.at(-1).path, last); await f.dispatch(f.$("library-prev")); assert.ok(f.$("library-page").textContent.includes("3"));
  await f.dispatch(f.$("refresh-library")); assert.ok(f.calls.at(-1).path.includes("after=0&limit=32"));
});
test("short page stops next and enable preserves metadata", async () => {
  const f = await library(); assert.equal(f.$("library-next").disabled, true); await f.dispatch(f.button("启用"));
  assert.deepEqual(JSON.parse(f.calls.find(c => c.method === "PATCH").body), { id: high, tags: asset().tags, emotion: "calm", description: asset().description, enabled: true });
});
test("cancel editor clears all metadata and cannot submit prior ID", async () => {
  const f = await library(); await f.dispatch(f.button("编辑")); f.$("library-form").elements.namedItem("enabled").checked = true;
  await f.dispatch(f.$("cancel-library")); const fields = f.$("library-form").elements;
  for (const name of ["tags", "emotion", "description"]) assert.equal(fields.namedItem(name).value, ""); assert.equal(fields.namedItem("enabled").checked, false);
  await f.dispatch(f.$("library-form"), "submit"); assert.equal(f.calls.filter(c => c.method === "PATCH").length, 0);
});
test("noncanonical and oversized metadata pages refuse safe rendering", async () => {
  for (const config of [{ pages: () => [asset("09007199254740993")] }, { pages: () => Array.from({ length: 33 }, (_, i) => asset(String(BigInt(high) + BigInt(i)))) }, { nextAfter: 9007199254740992 }, { pages: () => [asset(high), asset(high)] }]) {
    const f = await library(config); assert.equal(f.$("library-list").children.length, 0); assert.equal(f.$("library-next").disabled, true); assert.ok(f.$("notice").textContent.length > 0); assert.equal(f.calls.filter(c => c.method !== "GET").length, 0);
  }
});
test("delete cancellation and confirmed one-shot cleanup", async () => {
  const f = await library(); await f.dispatch(f.button("编辑")); await f.dispatch(f.button("预览")); const before = f.privateSnapshot();
  f.button("删除").dispatch("click"); await waitFor(() => f.$("confirm-dialog").open, "delete confirm"); f.$("confirm-cancel").dispatch("click"); await waitFor(f.idle, "cancel"); assert.deepEqual(f.privateSnapshot(), before); assert.equal(f.calls.filter(c => c.method === "DELETE").length, 0);
  f.button("删除").dispatch("click"); await waitFor(() => f.$("confirm-dialog").open, "delete confirm"); f.$("confirm-form").dispatch("submit"); await waitFor(f.idle, "delete");
  assert.equal(f.calls.filter(c => c.method === "DELETE").length, 1); assert.equal(f.$("library-editor").hidden, true); assert.equal(f.$("library-preview").children.length, 0);
});
test("preview fixed authenticated URL, load/error and stale-generation refusal", async () => {
  const f = await library(); await f.dispatch(f.button("预览")); const image = f.$("library-preview").children[0]; assert.equal(image.src, "/api/library/preview?id=" + high); assert.ok(!f.$("library-preview-status").textContent.includes("已通过"));
  const lateLoad = image.onload, lateError = image.onerror; image.onload(); assert.equal(f.$("library-preview-status").textContent, "本次预览已通过服务端校验"); image.onerror(); assert.ok(f.$("library-preview-status").textContent.includes("无法"));
  await f.dispatch(f.button("预览")); lateLoad(); lateError(); assert.ok(!f.$("library-preview-status").textContent.includes("已通过"));
  await f.dispatch(f.$("logout")); lateLoad(); lateError(); assert.equal(f.$("library-preview-status").textContent, ""); assert.equal(image.onload, null); assert.equal(image.onerror, null); assert.equal(image.getAttribute("src"), undefined);
});
for (const mode of ["success", "failure", "abort", "cancel"]) test("restore " + mode + " library dispatch invalidation", async () => {
  const f = await library({ restore: mode }); await f.dispatch(f.button("编辑")); await f.dispatch(f.button("预览")); const late = f.$("library-preview").children[0].onload; f.$("library-file").files = [{ size: 128 }]; await f.dispatch(f.nav("system"));
  const file = { size: 4096 }; f.$("restore-file").files = [file]; const before = f.privateSnapshot(); f.$("restore-form").dispatch("submit"); await waitFor(() => f.$("confirm-dialog").open, "restore confirm");
  f.$(mode === "cancel" ? "confirm-cancel" : "confirm-form").dispatch(mode === "cancel" ? "click" : "submit");
  if (mode === "abort") { await waitFor(() => f.dispatchSnapshot !== undefined, "restore dispatched"); f.$("cancel-request").dispatch("click"); } await waitFor(f.idle, "restore result");
  if (mode === "cancel") { assert.deepEqual(f.privateSnapshot(), before); assert.equal(f.calls.filter(c => c.path === "/api/restore").length, 0); }
  else { assert.deepEqual(f.dispatchSnapshot, { rows: "", page: "", tags: "", editor: true, files: 0, preview: 0, previewStatus: "" }); late(); assert.equal(f.$("library-preview-status").textContent, ""); assert.equal(f.calls.filter(c => c.path === "/api/restore").length, 1); assert.equal(f.calls.find(c => c.path === "/api/restore").body, file);
    await f.dispatch(f.$("library-form"), "submit"); assert.equal(f.calls.filter(c => c.method === "PATCH").length, 0); await f.dispatch(f.nav("library")); assert.ok(f.calls.at(-1).path.includes("after=0&limit=32")); }
});
test("logout and 401 clear all library private fields", async () => {
  for (const expire of [false, true]) { const f = await library(); await f.dispatch(f.button("编辑")); await f.dispatch(f.button("预览")); f.$("library-file").files = [{ size: 128 }]; if (expire) f.config.readStatus = 401;
    await f.dispatch(f.$(expire ? "refresh-library" : "logout")); assert.deepEqual(f.privateSnapshot(), { rows: "", page: "", tags: "", editor: true, files: 0, preview: 0, previewStatus: "" }); assert.equal(f.$("console-view").hidden, true); }
});
test("failed and aborted upload never retry or present known success", async () => {
  for (const write of ["failure", "abort"]) { const f = await library({ write }); const file = { size: 128 }; f.$("library-file").files = [file]; f.$("library-upload-form").dispatch("submit");
    if (write === "abort") { await waitFor(() => f.calls.some(c => c.method === "POST"), "upload dispatch"); f.$("cancel-request").dispatch("click"); } await waitFor(f.idle, "upload result"); assert.equal(f.calls.filter(c => c.method === "POST").length, 1); assert.equal(f.$("library-file").files[0], file); assert.ok(!f.$("notice").textContent.includes("已上传")); }
});
test("recovery responses to save and delete never retry or claim success", async () => {
  const f = await library({ write: "failure" }); await f.dispatch(f.button("编辑")); await f.dispatch(f.$("library-form"), "submit");
  assert.equal(f.calls.filter(c => c.method === "PATCH").length, 1); assert.ok(!f.$("notice").textContent.includes("已保存")); assert.equal(f.$("library-editor").hidden, false);
  f.button("删除").dispatch("click"); await waitFor(() => f.$("confirm-dialog").open, "delete confirm"); f.$("confirm-form").dispatch("submit"); await waitFor(f.idle, "delete recovery");
  assert.equal(f.calls.filter(c => c.method === "DELETE").length, 1); assert.ok(!f.$("notice").textContent.includes("已完成")); assert.equal(f.$("library-editor").hidden, false);
});
test("FIX1 backend-valid whitespace row renders beside ordinary row and previews", async () => {
  const f = await library({ pages: () => [{ ...asset(), tags: [" "] }, asset(String(BigInt(high) + 1n))] });
  assert.equal(f.$("library-list").children.length, 2, "backend-valid whitespace metadata cannot block whole page");
  assert.ok(f.$("library-list").textContent.includes(high)); assert.ok(f.$("library-list").textContent.includes(String(BigInt(high) + 1n)));
  await f.dispatch(f.button("预览")); const image = f.$("library-preview").children[0]; assert.equal(image.src, "/api/library/preview?id=" + high); image.onload(); assert.ok(f.$("library-preview-status").textContent.includes("已通过"));
});
test("FIX1 toggles preserve exact existing whitespace tags including disable", async () => {
  for (const enabled of [true, false]) {
    const f = await library({ pages: () => [{ ...asset(), tags: [" "], enabled }] }); await f.dispatch(f.button(enabled ? "禁用" : "启用"));
    assert.deepEqual(JSON.parse(f.calls.find(c => c.method === "PATCH").body), { id: high, tags: [" "], emotion: asset().emotion, description: asset().description, enabled: !enabled });
    assert.equal(f.calls.filter(c => c.method === "PATCH").length, 1);
  }
});
test("FIX1 existing whitespace may be edited and explicitly corrected", async () => {
  const f = await library({ pages: () => [{ ...asset(), tags: [" "] }] }); await f.dispatch(f.button("编辑")); const fields = f.$("library-form").elements;
  assert.equal(fields.namedItem("tags").value, " "); assert.equal(fields.namedItem("description").value, asset().description);
  await f.dispatch(f.$("library-form"), "submit"); assert.equal(f.calls.filter(c => c.method === "PATCH").length, 0, "new explicit Save keeps stricter nonblank policy");
  fields.namedItem("tags").value = "corrected"; await f.dispatch(f.$("library-form"), "submit"); assert.deepEqual(JSON.parse(f.calls.find(c => c.method === "PATCH").body).tags, ["corrected"]);
});
test("FIX1 existing whitespace may be deleted with honest rollback wording", async () => {
  const f = await library({ pages: () => [{ ...asset(), tags: [" "] }] }); const deleteButton = f.button("删除"); assert.ok(deleteButton, "backend-valid whitespace row exposes deletion"); deleteButton.dispatch("click"); await waitFor(() => f.$("confirm-dialog").open, "delete confirm");
  assert.ok(f.$("confirm-description").textContent.includes("删除当前范围的图片记录；用于恢复的旧内容可能保留")); assert.ok(!f.$("confirm-description").textContent.includes("删除其元数据与本机内容"));
  f.$("confirm-form").dispatch("submit"); await waitFor(f.idle, "delete result"); assert.equal(f.calls.filter(c => c.method === "DELETE").length, 1);
});
test("FIX1 delete confirmation describes scoped deletion and retained recovery bytes", async () => {
  const f = await library(); f.button("删除").dispatch("click"); await waitFor(() => f.$("confirm-dialog").open, "delete wording");
  const wording = f.$("confirm-description").textContent;
  f.$("confirm-cancel").dispatch("click"); await waitFor(f.idle, "cancel delete wording");
  assert.ok(wording.includes("删除当前范围的图片记录；用于恢复的旧内容可能保留"), "confirmation must not promise physical deletion of rollback content");
  assert.equal(f.calls.filter(c => c.method === "DELETE").length, 0);
});
test("FIX1 response admission retains null empty control and bound refusal", async () => {
  for (const metadata of [{ tags: null }, { tags: [""] }, { tags: ["bad\0"] }, { tags: ["bad\u0085"] }, { tags: ["x".repeat(33)] }, { emotion: "bad\u007f" }, { description: "bad\0" }, { enabled: "false" }]) {
    const f = await library({ pages: () => [{ ...asset(), ...metadata }] }); assert.equal(f.$("library-list").children.length, 0); assert.equal(f.calls.filter(c => c.method !== "GET").length, 0); assert.ok(f.$("notice").textContent.length > 0);
  }
});
(async () => { let passed = 0, failed = 0; for (const [name, fn] of tests) { try { await fn(); passed++; console.log(JSON.stringify({ name, result: "pass" })); } catch (error) { failed++; console.log(JSON.stringify({ name, result: "fail", message: error.message, stack: error.stack })); } } console.log(JSON.stringify({ passed, failed, browser: false, backend: false, network: false })); process.exitCode = failed ? 1 : 0; })();
