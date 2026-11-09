# Scan and import recovery

Library scans and initial, scheduled, watcher, and BookDrop imports share a persistent SQLite job queue. One worker processes jobs serially. Within each scan, two bounded preparation workers hash and prepare new metadata/covers; one writer rechecks matches and commits files in inventory order. Heavy archive/PDF/ebook/audio extraction is limited to one operation across the shared scanner, including metadata refreshes. File imports are explicitly partial and never reconcile missing files elsewhere in a library.

In Settings → Jobs:

- **Pause** holds queued work immediately, or finishes the current catalog write and checkpoints it before pausing. Uncommitted preparations are discarded for retry. Other queued jobs may proceed. A paused job stays paused across restarts and reserves its scan deduplication key.
- **Resume** resumes a paused, failed, or cancelled scan using its original library and paths. Resume can fail if a newer scan already reserves the same key; use the newer scan or cancel it first.
- **Cancel** stops after the current file and retains completed imports and checkpoints. It does not undo library changes. A cancelled job can be resumed or a new scan started.
- **Pausing** means hashing/extraction/indexing of the current file is still finishing. These operations are not forcibly interrupted by a user pause/cancel request.

On shutdown the worker stops claiming jobs and gets up to eight seconds to finish its current file. If the deadline expires, the process can exit and retry unfinished work after restart. SQLite rolls back unfinished transactions. On startup, interrupted running jobs are requeued, interrupted pause requests become paused, and cancellation requests become cancelled. Manually paused jobs never auto-start.

Each successful file stores a checkpoint with path, size, nanosecond modification time, and outcome. Resume inventories the paths again, then reuses checkpoints whose file and catalog state still match. Changed, failed, incompletely indexed, and uncheckpointed files are retried. The directory inventory itself is rebuilt; a large archive can spend time inventorying before the displayed processing count catches up. Checkpoints do not assume files remain unchanged while a job is paused.

New book, file, and metadata records commit in one transaction. Audio and search indexing can be retried after that commit. Cover files are derived disk assets, not part of the SQLite transaction. A file still changing while it is read is reported for retry. Watcher events arriving during a file import request another pass instead of disappearing into job deduplication.

A full scan reconciles missing records only after complete, successful processing and inventory. An unreadable or absent root fails the scan without mass missing-file changes. Reconciliation itself commits atomically and rechecks root accessibility before committing. A directory that still exists but is unexpectedly empty cannot always be distinguished from intentional file removal; mount readiness remains an external storage concern.

## Upgrading during an older scan

These changes are not active until the new application starts. Do not restart a currently running scan just to enable the new controls. Let it finish first when possible.

Migration 31 adds checkpoints, restart recovery tracking, follow-up scan requests, and an indexing completion marker. It does not remove library records. Existing running scan jobs have no new checkpoints yet: after upgrade they inventory again and use existing file hashes, sizes, modification times, and metadata to skip completed work. An unfinished file may be retried; this is recovery of a scan pass rather than an exact continuation of its old displayed counters.

Validation uses temporary databases and library files, including an abrupt subprocess exit, pause/resume, rollback injection, incomplete inventories, missing paths, changed timestamps, and BookDrop/watcher recovery. It does not restart the deployed container.

## Scan resource limits and timings

`scan.preparation_workers` in `config.yaml` defaults to `2`; set it to `1` for a slower disk or to `3`/`4` after measuring spare capacity. Values outside 1–4 are rejected (zero uses the default). No live configuration or deployed container is changed by editing the source. Apply configuration on a later restart, preferably after any older scan finishes.

The bound includes preparations waiting for the writer, so extracted covers cannot accumulate across a large library. Existing-file metadata repair remains on the serial writer path and shares the extraction limiter. Hashing checks cancellation between reads; an extraction already running may finish before pause completes. A forced shutdown still relies on durable checkpoints and transactional catalog writes.

Jobs record inventory, hashing, metadata/extraction, cover preparation, writer, checkpoint, and elapsed milliseconds for the latest scan attempt. Stage work is cumulative and may overlap; it is not a partition of elapsed time. Writer time includes any serial metadata repair or fallback preparation. On resume, timing resets for the new attempt while file outcome counts are reconstructed from checkpoints. No disk-specific speedup is assumed.

The per-file artificial sleep has been removed. Resume loads checkpoint records in one query rather than querying each file repeatedly. Missing-file reconciliation still waits until the successful end of the scan.
