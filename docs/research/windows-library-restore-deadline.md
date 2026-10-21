# Windows larger-library restore deadline observation

2026-10-21. Package45 remains open; engineering **94%=47/50**. Product targets remain native Linux amd64/arm64. This records a real Windows development-host limitation rather than a completed recovery gate.

Primary reverified the owner's manually downloaded synthetic bundle:169290bytes, magic valid, SHA256`24b6af7db2fbe09cb9c78c58f7db01fdb19533a62ac530de893fc384e8f58d65`. The supported browser file chooser selected it in a fresh offline35-image fixture. Automatic approval first refused the restore button; the owner then explicitly approved this isolated synthetic recovery. Opening a second same-session tab rotated CSRF, so the first confirmed request was explicitly rejected403 before application admission. Primary refreshed session and diagnostics before manual continuation; no unknown write was repeated. The later actual restore returned408. No further restore was submitted after that timeout, and no success, installation, prior-retained flag or restore-triggered session invalidation is inferred.

Post-timeout diagnostics showed stopped. Current UI cleared the old fact editor before dispatch as designed; this does not prove restoration. The fixture then joined run_ok/close_ok=true/external_delegate_calls0/process0; owned tabs closed and18789 refused connections. All synthetic files and the owner's downloaded bundle were retained. No actual chat database, authorization, paid provider or real channel was used.

A separate GPT-6.1 Sol Medium diagnostic used the same current Core/Processor APIs, no Web/network and independently generated tiny32x32 PNGs. Primary read its full harness, report and actual logs.

| Boundary |3 images|16 images|
|---|---:|---:|
|Export|1981ms|6909ms|
|Restore including synchronous cleanup|7820ms|32203ms|
|Processor entries during restore|18 successful|91 entered, final entry rejected on expired context|
|Result|PreviousRetained/ChannelPaused true|context deadline exceeded; both false|
|Core.Close|56ms|96ms|
|External adapter calls|0|0|
|Process exit|0|1, reproduced deadline|

Source and ordered wrapper events identify six per-image validation passes for same-library restore: staged preparation, initial prepared check, current preflight, retained-current copy, incoming validation and final prepared check. With16 images the first five complete and the final pass reaches its11th entry after expiry. Input EOF was reached about753ms after restore entry. The observed timeout therefore occurs in final prepared-image validation before the trusted database installer callback, rather than while reading the bundle. This does not identify individual Windows syscall/antivirus cost, demonstrate a safe optimization or prove failure on Linux.

No product deadlines, security validation, schema or SQLite strategy changed. The accepted server resource tool's complete-bundle roundtrip contains one image and cannot resolve16/35-image behavior. A concrete bounded target-Linux35-image browser comparison is being prepared, with new upload/run approval required. The Windows result remains a disclosed limitation even if a later native test passes. Existing minor401status wording and unexercised browser workflows remain separately recorded; no final45/49/50 acceptance is claimed here.
