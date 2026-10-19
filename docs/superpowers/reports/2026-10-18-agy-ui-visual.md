# MiskoAI 本机管理面板视觉更新报告 (`UI_REPORT.md`)

## 1. 核心视觉变更

### 色彩体系（暖白表面、墨色文本与柔和蓝强调色）
- **表面与层级**：将原先偏冷绿的底色（`#f5f7f4` / `#ecf1ec`）替换为温和克制的暖白与米灰层级：页面背景 `--bg-page: #faf8f5`、侧边栏 `--bg-sidebar: #f4f1eb`、卡片表面 `--bg-surface: #ffffff`、次级内嵌块 `--bg-elevated: #fcfbf9` 与暖灰分割线 `--line: #e4dfd7`。
- **墨色文本与对比度**：正文采用深墨色 `--ink: #1e242b`，次级标题与标签采用 `--ink-secondary: #3d4652`，辅助说明采用 `--muted: #5c6672`，确保在暖白与纯白表面上均具备良好的阅读对比度。
- **柔和蓝强调色**：将主强调色更新为克制的钢青蓝 `--accent: #2b5c8f`（悬停 `--accent-hover: #214973`、浅底色 `--accent-soft: #edf3f9`），应用于主按钮、当前导航项、品牌标识、表单聚焦环与信息提示。

### 字体排印与中文阅读节奏
- **本机中文字体栈**：采用 `-apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", "Segoe UI", system-ui, sans-serif`，无任何远程字体或外部资源请求。
- **层级与间距优化**：统一 `.page-heading`、`.card-heading`、`.card`、`.record` 的内外边距与分割线节奏；为 `.meta`、`.pager` 与 `.key-values dd` 增加 `font-variant-numeric: tabular-nums`，使计数、时间戳与配额数据对齐更工整。
- **长 ID 与哈希折行保护**：在 `.preserve`、`.meta`、`.metric .value`、`.metric .note`、`.key-values dd`、`.field-state`、`.record h3`、`#library-editor h2`、`#fact-editor h2` 等元素上明确配置 `overflow-wrap: anywhere` 与 `word-break: break-word`，防止 64 位 SHA256、RFC3339 时间戳或长 URL 撑破卡片与网格。

### 按钮、告警面板与对话框层级
- **按钮层级**：
  - **主操作（`button.primary`）**：柔和蓝实心背景配白色文字，用于保存、上传、登录等核心提交。
  - **次要操作（默认 `button`）**：白底配暖灰实线边框（`--line-strong`），悬停呈现浅蓝灰过渡。
  - **危险操作（`button.danger`）**：暖红浅底（`#fdf3f2`）配深红文字（`#b03028`）与边框，与常规按钮明显区分。
  - **轻量操作（`button.subtle`）**：顶栏透明底按钮，降低日常干扰。
  - **禁用与忙碌态（`:disabled` / `main[aria-busy="true"]`）**：降低不透明度并区分 `not-allowed` 与请求处理中的 `wait` 光标。
- **提示与危险区域**：区分普通信息横幅（`.banner` / `.hint`）、会话校验警告（`#session-warning.warning` 暖黄背景与深琥珀文字）、错误消息（`.error`）以及高风险维护卡片（`.danger-zone` 淡红背景与警示标题）。
- **确认对话框（`dialog#confirm-dialog`）**：优化模态框阴影、半透明背景遮罩、描述文本独立边框容器（支持超长文本内部滚动）以及底部右对齐操作区。

### 响应式布局与无障碍保持
- **桌面端（1280px）与移动端（375px）适配**：
  - 桌面端保持 248px 固定侧栏与自适应主工作区，4 列指标卡、3 列档案网格、双列详情卡节奏舒展。
  - 平板端（`<=1080px`）自动收敛为 2 列指标/档案与单列详情。
  - 移动端（`<=760px` 及 `<=420px` / `375px`）将侧栏转为顶部紧凑双列/三列导航网格，表单、键值对（`.key-values`）与指标卡在窄屏下平滑转为单列堆叠，避免水平溢出。
- **无障碍与交互契约**：完整保留 `[hidden] { display: none !important; }`、`.skip` 键盘跳转链接、`.sr-only`、`:focus-visible` 清晰聚焦环，并新增 `@media (prefers-reduced-motion: reduce)` 规则以尊重系统减弱动态效果设置。

## 2. 边界与已知限制

1. **只读 HTML/JS 约束下的结构保持**：根据任务规范，`index.html` 与 `app.js` 保持完全只读，所有视觉优化均通过纯 CSS 选择器完成，未改动任何 DOM 结构、文案、类名、ID 或前端交互逻辑。
2. **动态注入节点的样式覆盖**：`app.js` 动态生成的候选引用（`blockquote`）、事件记录（`#events .record > span`）及空状态（`#library-preview:empty`）已通过结构化 CSS 选择器补充样式，但无法在不改动 JS 的前提下为部分内联文本片段拆分额外的状态徽标 DOM。
3. **纯静态样式交付**：本次修改仅使用文件编辑工具更新 `style.css` 与新增本报告，未执行任何命令行、浏览器渲染抓取或外部网络请求。

## Primary review corrections (2026-10-18)

- M1: Removed sticky positioning from the topbar. Editor cards use existing block:start jumps; the header now remains in document flow and cannot overlay their titles. Original draft static geometry risk retained; the first desktop library fixture did not reproduce an overlap because the page bottom limited scrolling.
- M2: Reduced-motion CSS disables CSS transitions/animations. Existing explicit JavaScript smooth editor scrolling is unchanged and is not overridden by this CSS; complete reduced-motion behavior is not claimed.
- The initial AGY task performed no browser verification; primary synthetic authenticated browser regression is a separate evidence gate.

## Primary local acceptance, 2026-10-19

Engineering **78%** remains39/50. Actual installed agy1.4.2 ran once as gemini-4-argon-medium / effort medium, exit0. Full draft and one root CSS/report correction received independent gpt-6.1-sol/medium review: initial0Critical/0Important/2Minor; FIX1 both addressed,0remaining/0new. No HTML/JavaScript change.

Primary verified all6 UI and473 complete fixture identities after testing, zero mismatch; actual Node regressions web4/library23/bundle27 passed, all exits0. Real synthetic authenticated browser tested viewport1280x900 (client1265) and375x812 (client360 including native scrollbar): static header, visible-only active section, dashboard, mobile system warnings, library page2 exact34/35/9007199254740993, longSHA editor wrap without horizontal overflow, hostile strings inert, modal328px inside360px client and cancellation closes it without deleting. Actual images of desktop/mobile/editor retained privately. These are selected UI regression checks, not complete final package45. Preview/upload/recovery are covered by prior source-bound gates and are not newly claimed here.

Both new synthetic fixtures used actual Core/SQLite/Manager/Web, no Core.Run, rejecting counted external delegates and closed CDN. Initial was explicitly quit; FIX1 reached its finite30minute deadline across the interrupted session. Both actually joined run_ok=true/close_ok=true/external_delegate_calls=0/process0. Missing English accessible navigation labels on mobile were corrected after reading actual DOM; original no-match probe retained. First ambiguous record substring matched11 buttons and refused before action; exact heading locator succeeded. Initial draft library scroll did not reproduce overlap because the page bottom limited its scroll. No actual private database, provider or WeChat/CDN call, paid batch, CI or final/resource acceptance. Existing explicit JS smooth scrolling remains a known CSS-scope limitation.
