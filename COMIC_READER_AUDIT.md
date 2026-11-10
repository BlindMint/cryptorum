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

Long-strip/infinite mode keeps native one-finger scrolling; pinch zoom is handled inside the comic content, keeping browser scale and navigation bars unchanged.
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

## Completion and seeking

Continuous strips mark the final page complete at the scroll boundary (including
negative RTL horizontal offsets), provided the final image is loaded. A strip
without overflow requires all its images to be loaded before this boundary rule
applies. A final paginated spread also reports completion.

Dragging the progress slider updates its thumb, label and accessible value
immediately without navigating every intermediate page. Releasing commits one
seek without a long smooth scroll. The selected page stays authoritative briefly
while images settle; wheel or pointer interaction returns control immediately.
Late image loads during normal reading no longer force a page-top jump.
Nearby pages load eagerly, placeholder aspect ratios reserve space, and scroll
page tracking runs at most once per animation frame.

Validation: scroll-boundary regression cases, plus mobile Chromium with 40 pages
and delayed image responses for immediate drag feedback, stable seeks, completion
and returning to manual scroll.


## Mobile pinch isolation

The comic reader prevents browser-level pinch zoom within its own surface and
handles two-finger touch zoom on comic content in every scroll mode. Navigation
bars and settings remain unscaled. Native one-finger scrolling remains enabled
for continuous strips, including RTL horizontal scrolling. Content dimensions
are measured in pixels so percentage sizing cannot cancel the image zoom.
Pinch midpoint positioning and Ctrl/Command-wheel use the same content-only
zoom path; the application-wide viewport configuration remains unchanged.

Validated with trusted Chromium mobile touch gestures for paginated, vertical,
horizontal and RTL modes, checking image growth, invariant visualViewport.scale
and navigation-bar dimensions, plus native one-finger scrolling. A physical
Android tablet was not available for this check.

## Paginated carousel

Paginated reading now renders adjacent viewport-sized frames in a horizontal
CSS snap carousel instead of replacing the current image. Native touch swipes
and mouse dragging expose adjacent pages during movement; arrows, edge taps and
keyboard navigation animate to a snap point. Progress seeks remain immediate.
Reduced-motion preferences disable page-turn animation.

Odd spreads pair pages 1–2, 3–4, etc.; even spreads leave the cover alone and
pair 2–3, 4–5, etc. Rendering, navigation, completion and seeking share this
grouping. Single final pages remain valid snap frames.

Zoom and panning stay inside the active frame with snap navigation suspended.
Controls remain outside scrolling/zoomed content. Horizontal Long Strip remains
free-scrolling and centers shorter images within the visible viewport,
including when zoomed out.

Validation includes grouping/RTL snap regression tests, trusted mobile swipes
with adjacent pages visible before release, edge-tap animation, cover/spread
navigation, progress completion, fixed arrow bounds, mouse drag, keyboard
navigation, zoom isolation, reduced motion and Long Strip centering.

## RTL progress controls

Both comic progress tracks follow reading direction. RTL fills originate at
right, and thumb, hover marker and tooltip mirror with them. Pointer seeks use
the actual track bounds with the logical beginning at right and end at left.
Left/right slider keys follow physical reading direction; Home/End select the
logical beginning/end. Labels retain their own text direction. Other readers
keep the shared progress component's default LTR direction.

## Arrow visibility and consecutive swipes

Global comic interface preferences expose separate desktop and mobile page-arrow
switches in the reader and app Settings. Desktop defaults on; mobile (viewport
width up to 768px) defaults off. These preferences remain global when a comic
has saved reading choices and use the existing local/backend settings storage.

Horizontal page swipes use pointer dragging with native vertical scrolling.
A new gesture cancels the running animation at the visible position and uses
its pending destination as the base for the next page turn. CSS snapping stays
disabled during the drag and its release glide, then resumes at rest. This
avoids browser snap ownership swallowing a second swipe or jumping on touch-down.
Each swipe still advances at most one page group. Wide pages pan inside their
frame, and pinch zoom retains its separate gesture handling.

Validation: preference migration/persistence tests, Svelte checks, production
build, and trusted Chromium mobile touch checks interrupting at 70% and 90% of
the snap distance in both LTR and RTL, verifying continuous touch-down position
and arrival at the following page. Pinch still zooms content without scaling the
browser. Physical-device feel still needs user feedback.

## Release glide

Page releases use one interruptible animation driven by animation frames with a
cubic ease-out. Release speed determines the duration, bounded to 140–420ms.
This avoids browser smooth scrolling easing in from rest after an active drag.
Scroll-end events cannot finish or restart the animation while it is running;
CSS snapping resumes only at the destination. Reduced motion remains immediate.

Validation: numerical trajectory tests cover monotonic deceleration, endpoints,
RTL and reverse motion, and fast-flick duration. Chromium frame samples confirm
steadily decreasing release speed in LTR and RTL. Late-glide interruption and
pinch-isolation checks also pass.
