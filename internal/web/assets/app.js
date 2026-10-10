"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const state = { csrf: "", authenticated: false, epoch: 0, busy: false, section: "dashboard", factID: null, profileID: null, facts: { offset: 0, next: 0, more: false, q: "" }, history: { offset: 0, next: 0, more: false, q: "" } };
  let activeRequest = null;
  let confirmation = null;
  const privateContainers = ["status-cards", "channel-status", "usage-status", "events", "settings-fields", "restart-state", "profile-list", "active-profile", "fact-list", "candidate-list", "derived-summary", "history-list", "diagnostics", "restore-result", "facts-page", "history-page"];
  const fields = [
    ["listen", "监听地址", "text"], ["deepseek_url", "DeepSeek 官方地址", "url"], ["model", "文本模型", "text"], ["vision_model", "视觉模型", "text"], ["weixin_url", "微信服务地址", "url"],
    ["max_output_tokens", "最大输出 Tokens", "number", 1, 4096], ["context_bytes", "上下文 Bytes", "number", 16384, 49152], ["context_tokens", "上下文 Tokens", "number", 4096, 65536]
  ];
  const builtinNames = { warm: "温暖自然", concise: "简洁直接", professional: "专业清晰" };
  function node(tag, text, className) { const n = document.createElement(tag); if (text !== undefined && text !== null) n.textContent = String(text); if (className) n.className = className; return n; }
  function text(id, value) { $(id).textContent = value ?? ""; }
  function empty(container, message = "暂无记录。") { container.replaceChildren(node("p", message, "empty")); }
  function pairs(container, entries) { const dl = node("dl", null, "key-values"); for (const [key, value] of entries) { dl.append(node("dt", key), node("dd", value ?? "不可用")); } container.append(dl); }
  function message(value, error = false) { const target = state.authenticated ? $("notice") : $("login-message"); target.textContent = value; target.classList.toggle("error", error); }
  function safeFailure(status) { return ({ 400: "输入或维护条件无效，请核对后重试。", 401: "会话已失效，请重新登录。", 403: "会话校验失败，请刷新会话后核对操作结果。", 404: "记录不存在或接口不可用，请刷新列表。", 408: "操作超时，请核对结果后再手动重试。", 409: "服务繁忙或记录已变化，请刷新后核对。", 429: "登录尝试过多，请稍后再试。", 503: "服务未配置或暂不可用。" })[status] || "服务未能完成操作，请核对状态后再试。"; }
  function resetPrivate() {
    for (const id of privateContainers) $(id).replaceChildren();
    for (const id of ["settings-form", "profile-form", "fact-form", "fact-search", "history-search", "restore-form"]) $(id).reset();
    state.factID = null; state.profileID = null;
    for (const key of ["facts", "history"]) state[key] = { offset: 0, next: 0, more: false, q: "" };
    text("fact-editor-title", "新增事实"); $("profile-form").elements.namedItem("id").readOnly = false;
    $("session-warning").hidden = true;
  }
  function forgetSession(reason = "") {
    state.epoch++; state.csrf = ""; state.authenticated = false;
    if (activeRequest) activeRequest.abort();
    if (confirmation) resolveConfirmation(false);
    resetPrivate(); $("password").value = ""; text("notice", "");
    $("console-view").hidden = true; $("login-view").hidden = false;
    text("login-message", reason);
  }
  function setBusy(busy) {
    state.busy = busy;
    for (const control of document.querySelectorAll("button,input,textarea,select")) {
      if (["cancel-request", "confirm-cancel", "confirm-accept"].includes(control.id)) continue;
      if (busy) { control.dataset.wasDisabled = control.disabled ? "yes" : "no"; control.disabled = true; }
      else if (control.dataset.wasDisabled !== undefined) { control.disabled = control.dataset.wasDisabled === "yes"; delete control.dataset.wasDisabled; }
    }
    $("cancel-request").hidden = !busy;
    $("main").setAttribute("aria-busy", String(busy));
  }
  async function run(task, success = "") {
    if (state.busy) return;
    setBusy(true); message("正在处理…"); const epoch = state.epoch;
    try { const result = await task(); if (result === false) message("已取消，未提交操作。"); else if (success) message(success); else message(""); }
    catch (error) { if (epoch === state.epoch || !state.authenticated) message(error instanceof Error ? error.message : "操作未完成。", true); }
    finally { setBusy(false); $("password").value = ""; }
  }
  async function boundedBody(response, limit, signal) {
    const rawSize = response.headers.get("Content-Length");
    let declared = null;
    if (rawSize !== null) {
      if (!/^(0|[1-9][0-9]*)$/.test(rawSize)) { if (response.body) await response.body.cancel(); throw new Error("下载大小声明无效。"); }
      declared = Number(rawSize);
      if (!Number.isSafeInteger(declared) || declared > limit) { if (response.body) await response.body.cancel(); throw new Error("响应超出允许大小，已停止接收。"); }
    }
    if (!response.body) throw new Error("响应流不可用，请使用支持流式读取的浏览器。");
    const reader = response.body.getReader(); const chunks = []; let total = 0;
    try {
      while (true) {
        if (signal.aborted) throw new Error("已取消等待。请核对操作结果后再手动提交。");
        const { value, done } = await reader.read(); if (done) break;
        total += value.byteLength;
        if (total > limit) throw new Error("响应超出允许大小，已停止接收。");
        chunks.push(value);
      }
      if (declared !== null && total !== declared) throw new Error("传输不完整，文件未下载。请重新核对服务状态。");
      return chunks;
    } catch (error) { try { await reader.cancel(); } catch (_) { /* closed stream */ } throw error; }
    finally { reader.releaseLock(); }
  }
  async function request(path, options = {}) {
    const method = options.method || "GET"; const mutation = method !== "GET";
    if (mutation && path !== "/api/login" && !state.csrf) throw new Error("请先刷新会话，核对操作结果后再提交。");
    const epoch = state.epoch; const controller = new AbortController(); activeRequest = controller;
    const timer = setTimeout(() => controller.abort(), 45000);
    const headers = {};
    if (mutation && path !== "/api/login") headers["X-MiskoAI-CSRF"] = state.csrf;
    let body;
    if (options.raw) { headers["Content-Type"] = "application/octet-stream"; body = options.raw; }
    else if (options.body !== undefined) { headers["Content-Type"] = "application/json"; body = JSON.stringify(options.body); }
    try {
      const response = await fetch(path, { method, headers, body, credentials: "same-origin", mode: "same-origin", cache: "no-store", redirect: "error", signal: controller.signal });
      if (epoch !== state.epoch) { if (response.body) await response.body.cancel(); throw new Error("会话已变化，请重新登录。"); }
      if (!response.ok) {
        if (response.body) await response.body.cancel();
        if (response.status === 401 && path !== "/api/login" && state.authenticated) forgetSession(safeFailure(401));
        if (response.status === 403 && state.authenticated) { state.csrf = ""; $("session-warning").hidden = false; }
        throw new Error(safeFailure(response.status));
      }
      const chunks = await boundedBody(response, options.download ? options.download.limit : 2 * 1024 * 1024, controller.signal);
      if (epoch !== state.epoch) throw new Error("会话已变化，请重新登录。");
      if (options.download) return new Blob(chunks, { type: options.download.type });
      const result = JSON.parse(await new Blob(chunks).text());
      return result;
    } catch (error) {
      if (controller.signal.aborted) throw new Error("已取消或超时。请刷新状态、核对操作结果后再手动提交。");
      if (error instanceof TypeError || error instanceof SyntaxError) throw new Error("连接中断或响应无效。请核对操作结果后再手动提交。");
      throw error;
    } finally { clearTimeout(timer); if (activeRequest === controller) activeRequest = null; $("password").value = ""; }
  }
  function validToken(value) { if (typeof value !== "string" || !/^[A-Za-z0-9_-]{43}$/.test(value)) throw new Error("会话响应无效，请重新登录。"); return value; }
  async function enter(csrf) {
    state.csrf = validToken(csrf); state.authenticated = true;
    $("login-view").hidden = true; $("console-view").hidden = false;
    $("session-warning").hidden = true; text("login-message", "");
    selectSection("dashboard"); await loadDashboard();
  }
  function selectSection(name) {
    state.section = name;
    for (const page of document.querySelectorAll(".page")) page.hidden = page.id !== name;
    for (const button of document.querySelectorAll("[data-section]")) { if (button.dataset.section === name) button.setAttribute("aria-current", "page"); else button.removeAttribute("aria-current"); }
  }
  function action(label, task, options = {}) {
    const b = node("button", label, options.danger ? "danger" : ""); b.type = "button";
    b.addEventListener("click", () => run(task, options.success || "操作已完成。")); return b;
  }
  function confirmOperation(title, description) {
    text("confirm-title", title); text("confirm-description", description);
    return new Promise((resolve) => { confirmation = resolve; $("confirm-dialog").showModal(); $("confirm-cancel").focus(); });
  }
  function resolveConfirmation(accepted) { const resolve = confirmation; confirmation = null; $("confirm-dialog").close(); text("confirm-description", ""); text("confirm-title", "确认操作"); if (resolve) resolve(accepted); }
  $("confirm-form").addEventListener("submit", (e) => { e.preventDefault(); resolveConfirmation(true); });
  $("confirm-cancel").addEventListener("click", () => resolveConfirmation(false));
  $("confirm-dialog").addEventListener("cancel", (e) => { e.preventDefault(); resolveConfirmation(false); });
  function metric(label, value, note) { const n = node("article", null, "metric"); n.append(node("span", label, "label"), node("strong", value, "value"), node("span", note, "note")); return n; }
  function channelLabel(value) { return ({ unconfigured: "通道未配置", provider_unconfigured: "模型服务未配置", restore_paused: "恢复后暂停", receiving: "接收中", backoff: "退避等待", stopped: "已停止", authorization_expired: "授权失效", paused: "已暂停", quarantined: "已隔离", closed: "已关闭", closing: "关闭中", unavailable: "不可用" })[value] || "未知状态（请查看安全状态码）"; }
  async function loadDashboard() {
    const s = await request("/api/status");
    $("status-cards").replaceChildren(metric("通道状态", channelLabel(s.channel_state), s.channel_state), metric("Go 堆内存", `${(s.heap_bytes / 1048576).toFixed(1)} MiB`, "RSS 不可用；非进程总内存"), metric("运行时间", `${Math.floor(s.uptime_seconds / 60)} 分钟`, "当前进程"), metric("Goroutines", s.goroutines, `GC 次数 ${s.gc_count}`));
    $("channel-status").replaceChildren();
    pairs($("channel-status"), [["DeepSeek 凭据", s.has_deepseek_key ? "已配置（不显示密钥）" : "未配置"], ["Ollama 凭据", s.has_ollama_key ? "已配置（不显示密钥）" : "未配置"], ["通道状态", s.channel.state], ["接收 / 处理 / 发送", `${s.channel.received} / ${s.channel.processed} / ${s.channel.sent}`], ["重复 / 失败 / 不明确", `${s.channel.duplicate} / ${s.channel.failed} / ${s.channel.ambiguous}`], ["通道最近状态码", s.channel.last_code || "无"], ["摘要完成 / 失败", `${s.summary.completed} / ${s.summary.failed}`], ["摘要最近状态码", s.summary.last_code || "无"]]);
    $("usage-status").replaceChildren();
    pairs($("usage-status"), [["聊天 / 摘要尝试", `${s.logical_attempts.chat} / ${s.logical_attempts.summary}`], ["视觉 / 搜索尝试", `${s.logical_attempts.vision} / ${s.logical_attempts.search}`], ["输入 / 输出 Tokens", `${s.successful_usage.prompt_tokens} / ${s.successful_usage.completion_tokens}`], ["成功总 Tokens", s.successful_usage.total_tokens], ["缓存 / 推理 Tokens", `${s.successful_usage.cached_tokens} / ${s.successful_usage.reasoning_tokens}`]]);
    $("events").replaceChildren(); if (!s.events.length) empty($("events"), "暂无安全事件记录。");
    else for (const event of s.events) { const r = node("div", null, "record"); r.append(node("span", event.kind), node("p", event.at, "meta")); $("events").append(r); }
  }
  // Read controls explicitly: pending controls are disabled to prevent duplicate
  // submissions, and native FormData would omit those disabled values.
  function formData(id) { return new Map(Array.from($(id).elements).filter((control) => control.name).map((control) => [control.name, control.value])); }
  function integer(value, min, max) { if (!/^(0|[1-9][0-9]*)$/.test(value)) throw new Error("数值必须是规范十进制整数。"); const n = Number(value); if (!Number.isSafeInteger(n) || n < min || n > max) throw new Error(`数值必须在 ${min}–${max} 之间。`); return n; }
  function boundedText(value, limit, label) { if (typeof value !== "string" || new TextEncoder().encode(value).length > limit) throw new Error(`${label}最多 ${limit} UTF-8 字节。`); return value; }
  function canonicalListen(value) { const match = /^(127\.0\.0\.1|\[::1\]):([1-9][0-9]{0,4})$/.exec(value); if (!match || Number(match[2]) > 65535) throw new Error("监听地址必须是 127.0.0.1:端口 或 [::1]:端口（1–65535，无前导零）。"); return value; }
  async function loadAI() {
    const s = await request("/api/settings");
    text("restart-state", s.restart_required ? "期望配置与当前生效配置不同，需要重启服务。" : "期望配置与当前生效配置一致。");
    const target = $("settings-fields"); target.replaceChildren();
    for (const [key, label, type, min, max] of fields) {
      const wrapper = node("label", label); const input = node("input"); input.name = key; input.type = type; input.required = true; input.value = s.desired[key];
      if (type === "number") { input.min = min; input.max = max; input.step = "1"; } else input.maxLength = key === "listen" ? 64 : 2048;
      const overridden = s.overrides[key] === true;
      wrapper.append(input, node("span", `当前生效：${s.effective[key]}\nENV 覆盖：${overridden ? "是 · 重启仍优先使用 ENV" : "否"}`, "field-state")); target.append(wrapper);
    }
    await loadProfiles();
  }
  async function loadProfiles() {
    const data = await request("/api/profiles"); text("active-profile", `当前档案：${data.active.name}（${data.active.id}）· 表达不授予权限`);
    const target = $("profile-list"); target.replaceChildren();
    for (const id of data.builtin_ids) { const r = node("article", null, "record"); r.append(node("h3", builtinNames[id] || id), node("p", `内置 · ${id}`, "meta")); const b = action(data.active.id === id ? "正在使用" : "使用档案", async () => { await request("/api/profiles/select", { method: "POST", body: { id } }); await loadProfiles(); }); b.disabled = data.active.id === id; r.append(b); target.append(r); }
    for (const p of data.custom) {
      const r = node("article", null, "record"); r.append(node("h3", p.name), node("p", p.description, "preserve"), node("p", `自定义 · ${p.id}`, "meta")); const controls = node("div", null, "actions");
      controls.append(action(data.active.id === p.id ? "正在使用" : "使用", async () => { await request("/api/profiles/select", { method: "POST", body: { id: p.id } }); await loadProfiles(); }), action("编辑", async () => editProfile(p)), action("删除", async () => { if (!await confirmOperation("删除表达档案", `删除自定义档案「${p.name}」？`)) return false; await request(`/api/profiles?id=${encodeURIComponent(p.id)}`, { method: "DELETE" }); if (state.profileID === p.id) clearProfile(); await loadProfiles(); }, { danger: true })); r.append(controls); target.append(r);
    }
  }
  function clearProfile() { $("profile-form").reset(); $("profile-form").elements.namedItem("id").readOnly = false; state.profileID = null; }
  function editProfile(profile) { for (const key of ["id", "name", "description", "style", "address", "length", "sticker", "humor"]) $("profile-form").elements.namedItem(key).value = profile[key]; $("profile-form").elements.namedItem("id").readOnly = true; state.profileID = profile.id; $("profile-editor").scrollIntoView({ block: "start", behavior: "smooth" }); }
  function stringID(value) { if (typeof value !== "string" || !/^[1-9][0-9]{0,18}$/.test(value) || value.length === 19 && value > "9223372036854775807") throw new Error("记录标识无效，请刷新列表。"); return value; }
  function pageQuery(page, limit) { return `?${new URLSearchParams({ q: page.q, offset: String(page.offset), limit: String(limit) })}`; }
  function pagerDisabled(button, disabled) { if (state.busy && button.dataset.wasDisabled !== undefined) button.dataset.wasDisabled = disabled ? "yes" : "no"; button.disabled = state.busy || disabled; }
  function updatePager(kind, data, limit) { const page = state[kind]; page.next = data.next_offset; page.more = data.has_more && data.next_offset <= 10000; pagerDisabled($(kind + "-prev"), page.offset === 0); pagerDisabled($(kind + "-next"), !page.more); text(kind === "facts" ? "facts-page" : "history-page", `第 ${Math.floor(page.offset / limit) + 1} 页${data.has_more && !page.more ? " · 已到分页上限，请缩小搜索范围" : ""}`); }
  async function loadFacts() {
    const data = await request("/api/facts" + pageQuery(state.facts, 16)); const target = $("fact-list"); target.replaceChildren();
    if (!data.items.length) empty(target, "当前范围没有匹配的已确认事实。");
    for (const f of data.items) { stringID(f.id); const r = node("article", null, "record"); r.append(node("p", f.content, "preserve"), node("p", `ID ${f.id} · ${f.category || "未分类"} · 重要性 ${f.importance} · 来源 ${f.source} · 置信度 ${f.confidence}`, "meta"), node("p", `更新 ${f.updated_at} · 到期 ${f.expires_at ?? "无"}`, "meta")); const controls = node("div", null, "actions"); controls.append(action("编辑 / 更正", async () => editFact(f)), action("删除", async () => { if (!await confirmOperation("删除已确认事实", `确定删除这条事实？\n${f.content}`)) return false; await request(`/api/facts?id=${encodeURIComponent(f.id)}`, { method: "DELETE" }); if (state.factID === f.id) clearFact(); await loadFacts(); }, { danger: true })); r.append(controls); target.append(r); }
    updatePager("facts", data, 16);
  }
  function clearFact() { state.factID = null; $("fact-form").reset(); text("fact-editor-title", "新增事实"); }
  function editFact(f) { state.factID = stringID(f.id); for (const key of ["content", "category", "importance", "expires_at"]) $("fact-form").elements[key].value = f[key] ?? ""; text("fact-editor-title", `编辑事实 · ${f.id}`); $("fact-editor").scrollIntoView({ block: "start", behavior: "smooth" }); }
  async function loadMemoryReview() {
    const data = await request("/api/memory"); const target = $("candidate-list"); target.replaceChildren();
    if (!data.candidates.length) empty(target, "暂无待确认候选。");
    for (const c of data.candidates) { stringID(c.id); const r = node("article", null, "record"); r.append(node("p", c.content, "preserve"), node("p", `原消息 ID：${c.message_id}`, "meta"), node("blockquote", c.quote, "preserve"), node("p", `候选 ID ${c.id} · ${c.created_at}`, "meta")); const controls = node("div", null, "actions"); controls.append(action("确认事实", async () => { await request("/api/memory/confirm", { method: "POST", body: { id: c.id } }); await loadMemoryReview(); await loadFacts(); }), action("拒绝", async () => { if (!await confirmOperation("拒绝记忆候选", `移除这条待确认候选？\n${c.content}`)) return false; await request("/api/memory/reject", { method: "POST", body: { id: c.id } }); await loadMemoryReview(); })); r.append(controls); target.append(r); }
    $("derived-summary").replaceChildren();
    if (!data.derived.text) empty($("derived-summary"), "暂无派生摘要。"); else { $("derived-summary").append(node("p", data.derived.text, "preserve"), node("p", `水位 ${data.derived.watermark} · 修订 ${data.derived.revision} · ${data.derived.updated_at}`, "meta")); }
  }
  async function loadHistory() {
    const data = await request("/api/history" + pageQuery(state.history, 8)); const target = $("history-list"); target.replaceChildren();
    if (!data.items.length) empty(target, "当前范围没有匹配的历史。");
    for (const h of data.items) { const r = node("article", null, "record"); r.append(node("p", `${h.created_at} · ID ${h.id}`, "meta"), node("h3", "收到的消息"), node("p", h.content, "preserve"), node("h3", "记录的回复"), node("p", h.reply || "暂无已记录回复", "preserve")); target.append(r); }
    updatePager("history", data, 8);
  }
  async function loadMemory() { await loadFacts(); await loadMemoryReview(); await loadHistory(); }
  async function loadSystem() { const d = await request("/api/diagnostics"); $("diagnostics").replaceChildren(); pairs($("diagnostics"), [["产品", d.product], ["Go 版本", d.go_version], ["系统 / 架构", `${d.os} / ${d.architecture}`], ["外部探测", d.external_probe === false ? "未执行" : "未知状态"], ["通道", `${channelLabel(d.status.channel_state)} · ${d.status.channel_state}`], ["进程 RSS", "不可用；不作资源验收结论"]]); }
  async function loadSection() { if (state.section === "dashboard") await loadDashboard(); else if (state.section === "ai") await loadAI(); else if (state.section === "memory") await loadMemory(); else await loadSystem(); }
  async function download(path, filename, limit, type) { const blob = await request(path, { method: "POST", body: {}, download: { limit, type } }); const url = URL.createObjectURL(blob); const link = node("a"); link.href = url; link.download = filename; document.body.append(link); try { link.click(); } finally { link.remove(); URL.revokeObjectURL(url); } }
  $("login-form").addEventListener("submit", (e) => { e.preventDefault(); const password = $("password").value; $("password").value = ""; run(async () => { const b = await request("/api/bootstrap"); const data = await request("/api/login", { method: "POST", body: { password, nonce: validToken(b.nonce) } }); await enter(data.csrf); }); });
  $("logout").addEventListener("click", () => { const token = state.csrf; forgetSession("已退出本页面。正在结束服务端会话…"); run(async () => { if (!token) throw new Error("本页面数据已清除；服务端会话将在到期后失效。可重新登录后再退出。"); state.csrf = token; try { await request("/api/logout", { method: "POST", body: {} }); } finally { state.csrf = ""; } }, "已退出登录，页面数据已清除。"); });
  $("cancel-request").addEventListener("click", () => { if (confirmation) resolveConfirmation(false); if (activeRequest) activeRequest.abort(); });
  $("session-refresh").addEventListener("click", () => run(async () => { try { const data = await request("/api/session"); state.csrf = validToken(data.csrf); $("session-warning").hidden = true; await loadSection(); } catch (error) { if (!state.csrf) forgetSession("会话不可用，请重新登录。"); throw error; } }, "会话已刷新；请核对状态后手动提交操作。"));
  for (const b of document.querySelectorAll("[data-section]")) b.addEventListener("click", () => run(async () => { selectSection(b.dataset.section); await loadSection(); $("main").focus(); }));
  document.querySelector(".brand").addEventListener("click", (e) => { e.preventDefault(); run(async () => { selectSection("dashboard"); await loadDashboard(); }); });
  for (const [id, task] of [["refresh-dashboard", loadDashboard], ["refresh-ai", loadAI], ["refresh-memory", loadMemory], ["refresh-system", loadSystem]]) $(id).addEventListener("click", () => run(task));
  $("settings-form").addEventListener("submit", (e) => { e.preventDefault(); run(async () => { const data = formData("settings-form"); const body = {}; for (const [key, , type, min, max] of fields) body[key] = type === "number" ? integer(data.get(key), min, max) : data.get(key); canonicalListen(body.listen); await request("/api/settings", { method: "POST", body }); await loadAI(); }, "期望配置已保存。请查看当前生效值和重启提示。"); });
  $("profile-form").addEventListener("submit", (e) => { e.preventDefault(); run(async () => { const data = formData("profile-form"); const body = {}; for (const key of ["id", "name", "description", "style", "address", "length", "sticker"]) body[key] = data.get(key); if (Object.hasOwn(builtinNames, body.id)) throw new Error("内置档案不可覆盖，请使用新的标识。"); for (const [key, limit, label] of [["name", 128, "名称"], ["description", 1024, "说明"], ["style", 512, "风格"], ["address", 128, "称呼"]]) boundedText(body[key], limit, label); body.humor = integer(data.get("humor"), 0, 3); await request("/api/profiles", { method: "POST", body }); clearProfile(); await loadProfiles(); }, "档案已保存；选择后用于下一条消息。"); });
  $("new-profile").addEventListener("click", () => { clearProfile(); $("profile-form").elements.namedItem("id").focus(); }); $("cancel-profile").addEventListener("click", clearProfile);
  $("fact-form").addEventListener("submit", (e) => { e.preventDefault(); run(async () => { const data = formData("fact-form"); const body = { content: boundedText(data.get("content"), 16384, "事实内容"), category: boundedText(data.get("category"), 256, "分类"), importance: integer(data.get("importance"), 0, 100), expires_at: data.get("expires_at").trim() || null }; const editing = state.factID !== null; if (editing) body.id = stringID(state.factID); await request("/api/facts", { method: editing ? "PATCH" : "POST", body }); clearFact(); await loadFacts(); }, "事实已保存。"); });
  $("new-fact").addEventListener("click", () => { clearFact(); $("fact-form").elements.content.focus(); }); $("cancel-fact").addEventListener("click", clearFact);
  for (const [kind, form, query, limit, load] of [["facts", "fact-search", "fact-query", 16, loadFacts], ["history", "history-search", "history-query", 8, loadHistory]]) {
    $(form).addEventListener("submit", (e) => { e.preventDefault(); run(async () => { const value = $(query).value; if (new TextEncoder().encode(value).length > 1024) throw new Error("搜索词最多 1024 UTF-8 字节。"); state[kind].q = value; state[kind].offset = 0; await load(); }); });
    $(kind + "-prev").addEventListener("click", () => run(async () => { state[kind].offset = Math.max(0, state[kind].offset - limit); await load(); }));
    $(kind + "-next").addEventListener("click", () => run(async () => { if (state[kind].more) { state[kind].offset = state[kind].next; await load(); } }));
  }
  $("export-memory").addEventListener("click", () => run(() => download("/api/export", "miskoai-memory.json", 64 * 1048576, "application/json"), "完整记忆文件已准备下载，请妥善保管。"));
  $("backup").addEventListener("click", () => run(() => download("/api/backup", "miskoai-backup.db", 256 * 1048576, "application/octet-stream"), "完整备份已准备下载，请妥善保管。"));
  $("clear-memory").addEventListener("click", () => run(async () => { if (!await confirmOperation("清空当前范围的记忆与历史", "这会删除当前固定范围的事实、候选、派生摘要与对话历史。请先导出或备份。确认执行？")) return false; await request("/api/memory/clear", { method: "POST", body: { acknowledge: "clear_memory" } }); clearFact(); await loadMemory(); }, "范围维护操作已结束，请核对列表。"));
  $("restore-form").addEventListener("submit", (e) => { e.preventDefault(); run(async () => {
    const file = $("restore-file").files[0];
    if (!file || file.size === 0 || file.size > 256 * 1048576) throw new Error("请选择非空且不超过 256 MiB 的备份文件。");
    if (!await confirmOperation("恢复所选备份", "将替换当前数据。请先下载当前备份。恢复后通道保持暂停，需人工核对历史后单独恢复通道。确认执行？")) return false;
    // Once dispatched, even an aborted response can leave a new database
    // generation installed. Discard all old edit targets before upload; keep
    // the chosen File locally, the session, busy controls and selected section.
    resetPrivate();
    for (const kind of ["facts", "history"]) { pagerDisabled($(kind + "-prev"), true); pagerDisabled($(kind + "-next"), true); }
    const result = await request("/api/restore", { method: "POST", raw: file });
    text("restore-result", `此前快照保留：${result.prior_retained === true ? "是" : "否"} · 通道暂停：${result.channel_paused === true ? "是" : "否"}。请人工核对历史后再单独恢复通道。`);
    await loadDashboard(); await loadAI(); await loadMemory(); await loadSystem();
  }, "恢复请求已完成，请查看结果；未自动恢复通道。" ); });
  $("resume-channel").addEventListener("click", () => run(async () => { if (!await confirmOperation("人工核对后恢复通道", "确认你已核对恢复后的接收、去重和发送历史，处理可能重复的记录，并允许恢复外部回复？")) return false; await request("/api/channel/resume", { method: "POST", body: { acknowledge: "reconciled_restored_history" } }); await loadSystem(); }, "通道操作已结束，请核对诊断状态。"));
  run(async () => { try { const data = await request("/api/session"); await enter(data.csrf); } catch (error) { forgetSession(error instanceof Error && error.message === safeFailure(401) ? "请输入管理密码。" : "会话未恢复，请登录管理面板。"); } });
})();
