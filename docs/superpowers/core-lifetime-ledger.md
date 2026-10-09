# Core lifetime progress ledger

Root owns integration and acceptance. Implements the serial Core lifetime plan after the locally reviewed configuration/privatefs and management-storage gates. Children use only GPT-6.1 Medium/Low and never delegate or touch actual private data.

Task1 maintenance: not started; Task2 Core: not started. Management-storage implementation is still in progress at preparation time.

Ruling: InstallRestore and ClearRestorePause consume the live Lock object, rather than accepting a directory and trusting the presence of a lock filename. Source preflight found no existing ownership API. This narrows the planned interface so nil/closed/replaced/wrong-directory ownership can be tested before installation; no restore is executed on actual data. If wrong, future controller calls require adaptation, but product/provider/SQLite/resource routes remain unchanged.

Root preflight: Store.Close is normal one-connection close; no exported explicit checkpoint. Replacement requires successful normal close and absence of unexplained journals. Candidate validation uses immutable ValidateBackup before quota-admitted writable candidate open. All streamed source paths validate raw spelling before normalization, opened identity before read and metadata/privacy again afterward. A blocked arbitrary Reader cannot be force-canceled by context alone; actual work must join and final HTTP upload read deadline is a separate boundary.

Root service preflight: Service.Run keeps its caller lifetime alive after canceling only its internal account context. Core must therefore observe actual ErrAuthExpired in its own poll/send adapter, cancel and join the shared generation including summary, and preserve a safe expiry reason while management remains available. Task2 plan adds a targeted synthetic expiry/join/management test; no service-package ownership change, polling monitor or real call is required.
