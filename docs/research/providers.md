# Official provider research

Retrieved 2026-10-09. Documentation review only: no credentials supplied, live provider requests made, or paid calls performed. Keep model IDs configurable; the identifiers and limits below describe the retrieved documentation, not permanent API guarantees.

## DeepSeek

- Base URL: `https://api.deepseek.com`; Chat Completions uses `POST /chat/completions` and Bearer API-key authentication. Current documented model IDs are `deepseek-flash` and `deepseek-v4-pro`. [Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)
- Streaming uses data-only SSE, `choices[].delta`, and a terminal `[DONE]` event. Text and reasoning fields must be handled separately. The final chunk carries request usage; earlier usage can be null. `stream_options.include_usage` requires `stream: true`. Usage includes prompt, completion, total, cached and optionally reasoning token accounting. Thinking defaults enabled; `reasoning_effort: "none"` disables it. [Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/)
- `deepseek-flash` supports native vision. User-message content arrays can contain text and `image_url` blocks using base64 data URLs or public HTTP(S) URLs. JPEG, PNG, GIF and WebP are supported. Inline/external images have a 32 MiB per-image limit; request bodies have a 48 MiB limit. Maximum image side is 8192 pixels, reduced to 4096 with at least 15 images. Images in system or assistant Chat Completion messages fail validation. The old vision experiment ID is retired and routed to the latest Flash model. [Vision](https://api-docs.deepseek.com/guides/vision/)
- `GET /models` supplies IDs, modality metadata, context/output limits and effort support. Use discovery where available instead of treating legacy IDs as current. [Model list](https://api-docs.deepseek.com/api/list-models/)
- Documented failures: 400 malformed format, 401 authentication, 402 balance, 422 parameters, 429 rate limiting, 500 server failure and 503 overload. Retry decisions still depend on operation safety and application deadlines. [Error codes](https://api-docs.deepseek.com/quick_start/error_codes/)

Native DeepSeek vision satisfies the requested provider choice; no alternate vision supplier is needed based on this review. Actual text, image, cancellation and streamed usage behavior remains unverified without authorized live tests.

## Ollama hosted search

Direct cloud REST access requires a free account and API key, with `Authorization: Bearer <key>`; no local daemon or local model is required.

- `POST https://ollama.com/api/web_search`: JSON `query` string required; optional `max_results` defaults to 5 and is capped at 10. Response `results` entries contain `title`, `url` and `content`.
- Optional `POST https://ollama.com/api/web_fetch`: JSON `url`; response contains `title`, `content` and `links`. This is a documented capability, not approval to add arbitrary page fetching.
- The reviewed page specifies no numeric daily quota, requests-per-minute allowance, query-length limit or error response schema. Do not invent those limits. Application bounds are separate engineering controls.

Source: [Ollama web search](https://docs.ollama.com/capabilities/web-search). Hosted search live authentication and results remain unverified.

## SQLite candidate

Package-authored documentation identifies `modernc.org/sqlite` as pure Go without cgo and lists Linux amd64 and arm64. Generated library documentation exposes FTS5 code and APIs. Verify an FTS5 virtual table and consistent backup with the selected pinned version before accepting runtime support; this research did not install a dependency or execute SQL. [Driver documentation](https://pkg.go.dev/modernc.org/sqlite), [generated library](https://pkg.go.dev/modernc.org/sqlite/lib).
