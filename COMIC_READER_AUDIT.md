# Comic reader audit

The shared comic reader handles CBZ, CBR, CB7, and CBT page images.

## Changes

- Added pointer pinch zoom and horizontal swipe navigation in paginated mode.
  Zoomed pages can be dragged; fit-width pages retain vertical scrolling.
  Ctrl/Command-wheel changes image zoom instead of turning pages.
  Selecting a fit mode resets zoom.
- Persisted one page-size preference from both the toolbar and settings; it
  applies immediately and on reopening.
- Kept spread images side by side on mobile, removed the gap that exceeded
  the available grid width, and allocated fit-page columns by image aspect.
- Corrected odd spread pairing and navigation between spread boundaries.
  RTL/manga mode reverses the visual order.
- Required a matching pointer down before a center tap and suppressed taps
  after a pinch. Removed the top bar transform animation during visibility changes.
- Excluded settings inputs from global keyboard shortcuts and let oversized
  pages scroll without the wheel turning pages.
- Corrected progress-track coordinate mapping to include both endpoints.
- Enabled CB7 and CBT using the existing external archive extraction path.
  Packaged installations include 7z; local installations need a compatible
  extraction tool (7z supports these formats).

## Validation and limits

Browser regression checks used three synthetic images with different aspect
ratios at desktop and mobile viewport sizes. Checked reopening with fit width,
spread bounds, center taps, Ctrl-wheel zoom, swipe navigation and two-pointer
pinch events. These are automated Chromium checks, not physical-device testing.

Backend regression tests create ZIP, TAR and 7z archives, check page counts,
natural ordering and extracted page bytes. CBR listing output is covered for
unrar, 7z and bsdtar. A real RAR fixture was not available for end-to-end CBR
extraction testing; this path uses the existing CBR extractor.

Long-strip/infinite mode keeps native scrolling and browser pinch behavior.
The stored panel-view, spread-handling and transition-sound fields have no
reader implementation or exposed controls; they are not verified features.
PDF comics use the separate PDF reader and are outside this archive-reader audit.

## Preference scope

The headerless reader settings panel and app Settings use the same global
comic defaults. Page size, scroll mode, reading direction and page spread use
single-select chips. The reader shows page spread only in paginated mode.

“Remember reading settings for this comic” snapshots page size, scroll mode,
reading direction, spread and strip width for the current book ID. Subsequent
reading changes affect that override. Appearance and interface behavior remain
global. “Use global defaults” removes the snapshot immediately.

Overrides are part of the existing reader-settings JSON, saved locally and
through the backend settings endpoint; no database migration is required.
Legacy manga mode becomes right-to-left reading direction. The former opening
size preference is folded into page size, and unused legacy Auto choices map
to fit-page / spread off.

Validation includes store tests for scope, persistence and migration, plus
mobile Chromium checks for chips, cross-comic isolation, reopening, shared
appearance and removing overrides.

## Continuous scroll directions

Infinite Scroll is vertical; Long Strip is horizontal. Horizontal strips honor
right-to-left reading direction, track progress along the horizontal axis and
align thumbnail/progress seeks with the page's leading edge. Vertical mouse
wheel input moves along a horizontal strip; native horizontal trackpad/touch
scrolling remains available. Fit changes preserve the relative position within
the current image on either axis.
