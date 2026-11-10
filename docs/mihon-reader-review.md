# Mihon reader review and Cryptorum opportunities

Reviewed the local checkout at /home/steven/git/mihon, commit
524fa53043f2f5b2321ea13027e44f093ab4245c (2026-11-09), against the current
Cryptorum working tree, including the recent pagination and glide changes.
This is a source review, not an Android runtime or physical-device comparison.
Only this document was added for the review; reader behavior was not changed.

The main value is transferable behavior and separation of responsibilities.
Kotlin itself is not a significant obstacle. Android touch dispatch, RecyclerView,
image decoders and renderer libraries require different browser implementations.
The checkout contains both traditional Android viewers and an optional newer
GPU viewer. Findings below distinguish those paths; they are observations about
this checkout, not claims about every Mihon release.

## Where the biggest gains are

| Order | Opportunity | Benefit | Relative effort |
| --- | --- | --- | --- |
| 1 | Page-local loading/error/retry | One failed image no longer interrupts the entire book | Small–medium |
| 2 | Deliberate zoom/navigation behavior | Read enlarged pages with taps/keys before turning; double-tap at the point of interest | Medium |
| 3 | Bounded loading and image retention | More predictable large-book and mobile memory behavior | Medium–large |
| 4 | Tap layouts with a visual guide | More comfortable one-handed reading and discoverable controls | Small–medium |
| 5 | Separate webtoon presentation | Gapless artwork versus continuous pages with spacing; predictable viewport steps | Medium |
| 6 | Smarter wide-page/spread treatment | Better manga double-page reading on phones and tablets | Medium–large |
| 7 | Series defaults and issue transitions | Less repeated setup between issues; easier continued reading | Medium–large |

Effort is a relative implementation judgment, not a delivery estimate. These
are proposed directions, not changes to enable automatically.

## 1. Keep image failures inside the page

**Observed:** Mihon has page-specific loading states, download progress and error
views. Retry targets that page's loader. Its reader progress update also avoids
persisting progress on a page in an error state. See [page holder][holder],
[page error/retry][errors] and [progress updates][progress].

**Cryptorum:** The comic pager's image error callback sets a global readerError.
The reader surface is conditional on that flag, so a failed image can replace
the entire reading surface, including when it was a neighboring image loading
ahead. Continuous images use the same global-error pattern. See
[the image callback][cbxerrors].

**Adaptation:** Model pending/loading/ready/error per source page. Keep its frame
size and show a retry button within the failed page. Keep neighboring pages and
navigation available. Reserve the full-reader error screen for opening the book
or its page list failing. A page skipped because it failed should not count as
successfully read merely because navigation reached its index.

Acceptance: fail a background image, the active image, and one side of a spread;
retry each without losing position or restarting the reading session. A failed
final image must not complete the book.

## 2. Pan before turning, and make zoom easier to use

**Observed:** The traditional pager's navigation checks whether the image can pan
in the requested direction before turning the page. Image panning is animated
and interruptible. Initial zoom alignment follows reading direction; landscape
pages can receive special zoom handling. Double-tap returns between fit and an
enlarged view. The strip viewer also supports double-tap zoom and bounds its
pan/scale. See [pager navigation][pan], [image zoom/pan][zoom] and
[strip gestures][strip]. The GPU path considers a running pan's destination when
handling another navigation command, rather than repeatedly targeting from a
partially animated position: [queued pan target][gpunav].

**Cryptorum:** Pinch, anchored wheel zoom and dragging already work. Page
navigation currently resets zoom through goToPage. Paginated zoom disables the
carousel, and double-tap is not exposed as a reader zoom action. See
[current navigation][cbx] and [pager gesture ownership][cbxpager].

**Adaptation:** Offer double-tap to zoom around the tapped point and tap again
to fit. Offer “Navigation pans zoomed pages first” for taps/keys. Turn only when
no more content remains in that direction. Define whether a boundary requires
a fresh gesture before turning; prefer that conservative behavior initially.
Keep zoom and page motion under a clear owner so one gesture never triggers both.

A double-tap also requires delaying confirmation of a single tap in its zone;
otherwise the first tap could turn the page before zoom starts. Mihon's
[gesture detector][gesture] explicitly arbitrates double taps, long presses,
movement and additional fingers. Browser thresholds should be tested in CSS
pixels rather than copied from Android device units.

## 3. Bound work around the reading position

**Observed:** The traditional pager holds a small adjacent-page window; the
webtoon viewer recycles page holders. The GPU path maintains a bounded page
cache, preloads three ahead/two behind, protects images participating in a turn
and defers their cleanup. Continuous GPU loading scales its window to the visible
content. [Viewer/cache lifecycle][gpu], [recycling][recycle].

The HTTP loader prioritizes the requested page before planning nearby work,
deduplicates downloads and cancels work outside its immediate loading window.
Its rolling chunks can eventually load the rest of the chapter when permitted;
it is not a strict fixed-size network cache. [HTTP loading][load].

**Cryptorum:** Paginated frames and continuous images are rendered for the whole
book, with eager/lazy hints. That is useful initial scheduling, but browser lazy
loading does not provide an application-controlled decoded-image memory budget.
Thumbnails add another set of image elements. Server CBZ responses already have
ETags and private caching; replacing those mechanisms is unnecessary.
[Existing server caching][cbxcache].

**Adaptation:** Start with a bounded window of assigned image sources, preserving
fixed placeholders for other pages. Prioritize the visible frame and its spread
partner, then likely next pages. Defer thumbnails until the drawer is opened.
Await decode for nearby images when appropriate, and ignore stale completions
after a book/seek change. Keep both the departing and arriving images retained
until the current glide finishes or is superseded.

Only introduce full DOM virtualization after profiling shows it is needed. It
must preserve strip dimensions, scroll position, slider seeking and accessible
navigation. Removing an image element or source does not guarantee immediate
browser memory reclamation; measure real memory behavior on target devices.

## 4. Treat webtoons as a distinct presentation

**Observed:** Mihon distinguishes a gapless Webtoon mode from continuous vertical
pages with gaps, and also has vertically paginated reading. The traditional
webtoon viewer advances about three quarters of a viewport for navigation;
the GPU continuous path uses half-page viewport movement. Those are two
implementation choices, not one universal Mihon constant. [Mode definitions][modes],
[traditional strip navigation][webtoonviewer] and [GPU viewport steps][gpucontinuous], [shared viewer interface][viewer].

The strip viewer suppresses a menu tap when the touch began during manual
scrolling. Its tracker uses the last page whose end is visible, and loading
can extend across chapter boundaries. [Tap suppression][strip] and
[page tracking][track].

**Cryptorum:** Infinite Scroll means vertical continuous reading, while Long
Strip means horizontal continuous reading. Preserve these existing choices;
Mihon's use of “long strip” should not silently change our terminology or axis.
The current strip tracker uses a viewport anchor near its leading edge and
already handles final-page completion. [Current strip tracking][cbxtrack].

**Adaptation:** Add a gapless option and page-gap spacing to vertical continuous
reading. Let taps/keys advance a fraction of the viewport with overlap instead
of skipping to the top of the next source image, which can be extremely tall.
A touch that stops a fling should not also open the menu. Keep displayed current
page, durable resume position and completed/read-through position conceptually
separate; do not replace our current anchor policy without testing long images
and reverse scrolling.

## 5. Make navigation discoverable without permanent arrows

**Observed:** Tap layouts are normalized regions mapping to menu, forward and
backward actions. Physical left/right zones resolve according to reading
direction. Layouts include edge zones, broad left/right regions, one-handed
patterns and disabled navigation, with inversion options. The visual guide
uses those same region definitions. [Navigation model][tap],
[settings controls][settings], [guide overlay][overlay].

**Adaptation:** A small initial set—current layout, edge-only, one-handed and
disabled—would complement our mobile arrow default. Render a “Show tap zones”
preview from the exact hit-test data, both when a user selects a layout and on
demand. Use clear logical “Next/Previous” labels in RTL. Keep swipe direction,
tap layout and reading direction separate concepts.

This is a practical way to improve comfortable reading without adding more
persistent on-page controls.

## 6. Separate source pages from displayed frames

**Observed:** Mihon can split a wide source image into two logical reading steps,
invert their order, or rotate it to fit. Its inserted half retains the source
page's index. The newer GPU path additionally supports dual-page presentation
based on viewport shape and gives wide source pages their own frame.
[Image processing][split], [inserted page identity][insert],
[wide-page detection][gpuspread].

**Cryptorum:** Odd/even pairing and RTL are already supported. Pairing currently
depends on page indices, independent of source image dimensions. The saved
spreadHandling/panelViewEnabled fields are not functioning features, as the
existing audit explains.

**Adaptation:** First support “wide images stand alone” and optional automatic
dual-page display in landscape. Then consider splitting a source spread on small
screens, with the correct manga reading order and an explicit inversion option.
Do not infer that every landscape image is a spread: covers and panoramic art
also exist. Explicit user choices should remain available.

A displayed half needs an identity such as source-page plus half, not a new
archive page number. Persist the original page and optional subpage; count a
split source page as read only after its second half. Seeking, completion,
bookmarks and thumbnails must share the same grouping rules. Obtain image
sizes before regrouping when possible; never reshuffle a spread mid-drag just
because a late image reveals its shape.

## 7. Preserve meaningful setting scopes

**Observed:** Reading mode and orientation can inherit the global default or be
saved for a manga series. In-reader settings identify “this series” separately
from viewer-specific/global controls, and show only controls relevant to the
active viewer. [Default resolution][scopes], [settings UI][settings].

**Cryptorum:** Global defaults and explicit per-comic snapshots already exist.
The override intentionally covers reading choices only, while arrow visibility
and other interface choices stay global. Preserve that useful behavior.
[Existing override keys][cbxsettings].

**Adaptation:** Series-level reading defaults would reduce repeated setup across
issues. A possible resolution order is app defaults, series choices, explicit
book override. Distinguish inherited values from a frozen snapshot in the UI.
Keep separate paginated/webtoon options where they describe different behavior,
especially crop borders, tap layout and image spacing. Do not change current
snapshot semantics as an incidental side effect of adding series scope.

## 8. Keep movement, data changes and persistence separate

**Observed:** The pager tracker defers chapter-list changes while dragging or
settling, applying them at idle to prevent jumps. The GPU path also protects the
outgoing image from eviction during a transition. [Idle updates][idle],
[transition retention][gpu].

**Adaptation:** Extend the lesson to archive manifests, spread regrouping,
late dimensions and viewport changes: ordinary loading should not reissue a
page-turn command. Represent gesture/motion state explicitly if the current
component grows, and let structural updates wait for a safe point. A newly
initiated gesture must remain able to cancel the glide immediately.

Mihon's exact animation physics cannot be recovered from the Kotlin wrappers
alone: the traditional path uses Android ViewPager/subsampling views, and the
new path delegates to the ca.mpreg WebGPU viewer dependency. Its configuration
and call sites are visible, but that dependency's gesture integrator is not
vendored in this checkout. [Dependency declaration][gpulib]. Keep our recently
improved glide unless measurement reveals another issue; copying a duration or
an easing name is not enough to reproduce Android feel.

Mihon debounces ordinary progress writes, writes completion immediately and
serializes writes beyond reader teardown. [Progress persistence][progress].
Cryptorum already has an IndexedDB outbox, serialized checkpoint flow, revision
conflict handling and session shutdown logic. Preserve those stronger web sync
requirements. Any extra debouncing should reduce network churn without losing
the latest local checkpoint. [Existing progress controller][cbxsync].

## 9. Useful finishing touches

- **Page actions:** A long-press menu for save, share/copy and set-cover is useful,
  with an explicit menu alternative for keyboard users. Feature-detect browser
  sharing/clipboard support. [Mihon page actions][actions].
- **Reading appearance:** Optional border crop, grayscale/inversion on images
  alone, and automatic page-derived background colors. Start with reversible,
  off-by-default choices; cropping can hide edge text. Prefer server-produced
  crop metadata or cached variants if pixel analysis becomes expensive.
  [Image configuration][zoom] and [reader preferences][prefs].
- **Series continuation:** An end-of-issue transition with the next issue's
  title/cover and an explicit Continue action. Mihon preloads adjacent chapters
  and can expose transition pages. Adapt to Cryptorum's actual series ordering;
  do not assume file order or silently mark intervening issues read.
  [Adjacent chapter loading][preloadchapter], [page tracking][idle].
- **Compact controls:** Optional page count while bars are hidden, and an optional
  side-mounted progress track for tablets. Mihon supports horizontal RTL and
  vertical navigation. Cryptorum already mirrors its tracks; that work should
  remain. [Navigator][navigator].
- **Motion preferences:** An explicit animation-off choice can complement reduced
  motion. Keep double-tap zoom timing distinct from page-turn motion. Mihon's
  double-tap animation-speed setting is not a swipe-snap-speed setting.
  [Global reader settings][appsettings].

One intentional difference to retain: Mihon's slider navigates during dragging;
Cryptorum previews while dragging and commits one seek on release. The latter
already fits our server-backed reader well and avoids unnecessary image work.
[Mihon slider callbacks][scrub].

## 10. Expensive or platform-specific ideas to defer

Mihon's static-page subsampling and webtoon strip decoding address real texture
and memory limits. It can release oversized off-screen strip bitmaps while
retaining layout dimensions. [Strip lifetime][strips], [decoder][decoder].
If large webtoon pages become a measured problem here, server-generated strips
or tiles with a stable dimensions manifest are a plausible web solution.
The current CBZ manifest caches archive entries, not decoded image dimensions.
[Current manifest][cbxmanifest]. Cropping an enormous original into canvas tiles
only after fully decoding it in the browser would not solve the initial decode
memory cost. This deserves its own design and fixtures.

GPU shaders, advanced page-turn effects, HDR handling and configurable scalers
exist in the newer path, but are much larger projects than improving the current
DOM/image reader. Native volume keys, system brightness, cutout handling and
Android orientation controls also do not translate directly to a browser app.
E-ink flash controls are a niche option, not a general mobile default.

## Suggested first implementation sequence

1. Page-local status/retry and final-page readiness checks.
2. Double-tap zoom and optional pan-before-turn, with explicit gesture ownership.
3. Tap layout choices and a preview driven by the same region data.
4. Bounded image-source loading, thumbnail deferral and stable dimensions.
5. Gapless vertical reading, viewport-step navigation and inertial-tap suppression.
6. Wide-page handling and series-level preferences/continuation as separate work.

For each step, retain trusted browser touch coverage for interruption, reversal,
RTL, spreads, pinch-to-pan transitions, reduced motion and fast consecutive
inputs. Add delayed/failed image fixtures and large-page/books for loading work.
A physical Android device is needed to compare final feel to Mihon itself.

No implementation was copied. If later work adapts source code, retain the
relevant source attribution and notices; the Mihon checkout's AGENTS.md also
explicitly asks for source/copyright/license headers on substantial adapted code.

[modes]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/setting/ReadingMode.kt:24>

[prefs]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/setting/ReaderPreferences.kt:19>

[scopes]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/setting/ReaderSettingsViewModel.kt:53>

[settings]: </home/steven/git/mihon/app/src/main/java/eu/kanade/presentation/reader/settings/ReadingModePage.kt:28>

[tap]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/ViewerNavigation.kt:55>

[overlay]: </home/steven/git/mihon/app/src/main/java/eu/kanade/presentation/reader/ReaderNavigationOverlay.kt:29>

[gesture]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/GestureDetectorWithLongTap.kt:35>

[pan]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/pager/PagerViewer.kt:258>

[zoom]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/ReaderPageImageView.kt:148>

[strip]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webtoon/WebtoonRecyclerView.kt:79>

[idle]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/pager/PagerPageTracker.kt:32>

[load]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/loader/HttpPageLoader.kt:62>

[errors]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/ReaderPageError.kt:22>

[holder]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/pager/PagerPageHolder.kt:93>

[recycle]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webtoon/WebtoonAdapter.kt:97>

[strips]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webtoon/WebtoonStripImageView.kt:63>

[decoder]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/data/coil/ImageDecoder.kt:106>

[split]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/PageImageProcessing.kt:12>

[insert]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/model/InsertPage.kt:3>

[gpu]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webgpu/WebGpuViewer.kt:321>

[gpuspread]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webgpu/WebGpuViewer.kt:588>

[gpunav]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webgpu/WebGpuViewer.kt:1951>

[gpulib]: </home/steven/git/mihon/gradle/libs.versions.toml:198>

[progress]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/ReaderViewModel.kt:643>

[preloadchapter]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/ReaderViewModel.kt:464>

[track]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webtoon/WebtoonPageTracker.kt:28>

[actions]: </home/steven/git/mihon/app/src/main/java/eu/kanade/presentation/reader/ReaderPageActionsDialog.kt:29>

[navigator]: </home/steven/git/mihon/app/src/main/java/eu/kanade/presentation/reader/components/ChapterNavigator.kt:66>

[scrub]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/ReaderActivity.kt:590>

[viewer]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/Viewer.kt:16>

[appsettings]: </home/steven/git/mihon/app/src/main/java/eu/kanade/presentation/more/settings/screen/SettingsReaderScreen.kt:28>

[cbx]: </home/steven/datashard/cryptorum/frontend/src/routes/(app)/reader/cbx/[bookID]/+page.svelte:771>

[cbxerrors]: </home/steven/datashard/cryptorum/frontend/src/routes/(app)/reader/cbx/[bookID]/+page.svelte:1297>

[cbxtrack]: </home/steven/datashard/cryptorum/frontend/src/routes/(app)/reader/cbx/[bookID]/+page.svelte:528>

[cbxpager]: </home/steven/datashard/cryptorum/frontend/src/lib/components/ComicPager.svelte:93>

[cbxsettings]: </home/steven/datashard/cryptorum/frontend/src/lib/utils/comic-settings.ts:3>

[cbxsync]: </home/steven/datashard/cryptorum/frontend/src/lib/services/reading-progress.ts:390>

[cbxmanifest]: </home/steven/datashard/cryptorum/backend/cmd/server/handlers_files.go:79>

[cbxcache]: </home/steven/datashard/cryptorum/backend/cmd/server/handlers_files.go:940>

[webtoonviewer]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webtoon/WebtoonViewer.kt:65>

[gpucontinuous]: </home/steven/git/mihon/app/src/main/java/eu/kanade/tachiyomi/ui/reader/viewer/webgpu/WebGpuViewerContinuous.kt:92>
