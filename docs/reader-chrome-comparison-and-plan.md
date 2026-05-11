# Reader Chrome Comparison and Implementation Plan

## Summary

Cryptorum's EPUB, PDF, and CBX readers currently expose an interactive progress bar attached to the bottom edge of the top navigation bar. Codex separates those concerns: top progress is primarily visual, while intentional navigation lives in bottom controls. Cryptorum should adopt that structure for non-speed readers, while keeping the speed reader's center tap as play/pause.

## Reader Comparison

| Reader | Codex behavior | Cryptorum current behavior | Planned Cryptorum behavior |
| --- | --- | --- | --- |
| EPUB/text | Top app bar has title, actions, and visual chapter progress. Bottom bar owns progress navigation. Center tap toggles chrome. | Top bar includes title/actions and an interactive progress strip. Center tap already toggles top chrome. | Top strip becomes visual-only. A thicker bottom progress bar handles seeking. Center tap toggles top and bottom chrome together. |
| PDF | Uses the same reader chrome principles as page-based readers, with deliberate bottom navigation and minimal accidental seek surfaces. | EmbedPDF toolbar plus close/fullscreen shell controls and an interactive top progress strip. Center tap already toggles chrome. | Keep EmbedPDF toolbar behavior, make top progress visual-only, and add bottom page slider. |
| CBX/comics | Page-based reading uses deliberate bottom page navigation. Tap zones and chrome behavior stay consistent with the normal reader. | Top bar includes page controls and interactive progress. Tap currently reveals the top bar rather than matching EPUB/PDF center-toggle behavior. | Add bottom page slider, make top progress visual-only, then align CBX center tap with EPUB/PDF chrome toggling. Keep page controls and reading direction behavior intact. |
| Speed reader | Center tap is play/pause. Left/right tap zones step or pause. While paused, progress/status controls sit near the bottom; large jumps use the word picker. | Center tap is play/pause and edge zones step/pause. Top bar is crowded with progress, WPM, word picker, settings, and fullscreen. Bottom footer only has previous/play/next. | Keep center tap as play/pause. Move progress, WPM, and word picker affordances to a bottom status strip. Keep playback controls above the strip. Simplify the top bar. |
| Audio | No direct Codex audio reader equivalent was found in the inspected code. | Audio already uses a central card with an explicit progress slider and playback controls; it does not have the accidental top progress problem. | Leave audio out of the primary implementation. Revisit later if the whole reader chrome system is redesigned. |

## Implementation Plan

1. Create a shared Svelte progress track component for visual and interactive reader progress.
2. Replace the EPUB, PDF, and CBX top progress strips with visual-only tracks.
3. Add bottom interactive progress tracks to EPUB, PDF, and CBX.
4. Update CBX tap behavior so center taps toggle chrome like EPUB/PDF.
5. Rework speed reader chrome so the top bar is minimal and the bottom area carries progress/status actions while preserving center play/pause.
6. Run frontend validation and build checks.

## Acceptance Criteria

- Top progress in EPUB/PDF/CBX cannot seek by click, drag, keyboard, or pointer capture.
- Bottom progress in EPUB/PDF/CBX supports the existing seek behavior.
- EPUB/PDF center tap toggles both top and bottom chrome.
- CBX center tap matches EPUB/PDF chrome toggling after the chrome refactor.
- Speed reader center tap remains play/pause.
- Speed reader progress, WPM, and word picker access are visible in the paused bottom area.
- Existing backend progress/session APIs remain unchanged.

