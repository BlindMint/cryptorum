<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { comicGlide } from '$lib/utils/comic-motion';
	import { comicPageGroups, comicGroupIndex, comicSnapIndex } from '$lib/utils/comic-pages';
	import type { CbxReaderSetting } from '$lib/stores/readerSettings';

	let { total, currentPage, spread, fitMode, rtl, zoom, width, height, aspectRatios, saturation, brightness,
		getPageUrl, onpagechange, oncontentchange, onimageload, onimageerror, transition = 'auto' }: {
		total: number; currentPage: number; spread: CbxReaderSetting['pageSpread']; fitMode: CbxReaderSetting['fitMode'];
		rtl: boolean; zoom: number; width: number; height: number; aspectRatios: Record<number, number>;
		saturation: number; brightness: number; transition?: ScrollBehavior;
		getPageUrl: (page: number) => string; onpagechange: (page: number) => void;
		oncontentchange: (node: HTMLDivElement | undefined) => void;
		onimageload: (page: number, event: Event) => void; onimageerror: () => void;
	} = $props();

	let carousel = $state<HTMLDivElement>();
	let contents = $state<(HTMLDivElement | undefined)[]>([]);
	let dragging = $state(false);
	let dragStart: { id: number; x: number; y: number; scroll: number; index: number; pan?: HTMLElement; lastX: number; lastTime: number; velocity: number } | null = null;
	let settleTimer: ReturnType<typeof setTimeout> | null = null;
	let animationFrame: number | null = null;
	function stopAnimation() {
		if (animationFrame !== null) cancelAnimationFrame(animationFrame);
		animationFrame = null;
	}
	let layoutKey = '';
	let requestedIndex: number | null = null;
	const groups = $derived(comicPageGroups(total, spread));
	const activeIndex = $derived(comicGroupIndex(groups, currentPage));
	const fitPage = $derived(fitMode === 'fit-page' || fitMode === 'automatic');

	function motion(behavior: ScrollBehavior): ScrollBehavior {
		return window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : behavior;
	}
	function align(index: number, behavior: ScrollBehavior, velocity = 0) {
		if (!carousel) return;
		stopAnimation();
		const left = index * carousel.clientWidth * (rtl ? -1 : 1);
		if (Math.abs(carousel.scrollLeft - left) < 1) {
			requestedIndex = null;
			carousel.scrollTo({ left, behavior: 'instant' });
			if (!dragStart) carousel.style.removeProperty('scroll-snap-type');
			return;
		}
		requestedIndex = index;
		if (motion(behavior) !== 'smooth') {
			carousel.scrollTo({ left, behavior: 'instant' });
			return;
		}
		carousel.style.scrollSnapType = 'none';
		const glide = comicGlide(carousel.scrollLeft, left, velocity);
		const started = performance.now();
		function frame(now: number) {
			if (!carousel) { animationFrame = null; return; }
			carousel.scrollTo({ left: glide.position(now - started), behavior: 'instant' });
			if (now - started < glide.duration) {
				animationFrame = requestAnimationFrame(frame);
			} else {
				animationFrame = null;
				settled();
			}
		}
		animationFrame = requestAnimationFrame(frame);
	}
	function settled() {
		if (settleTimer) clearTimeout(settleTimer);
		settleTimer = null;
		if (!carousel || animationFrame !== null || dragStart || dragging || zoom !== 1) return;
		if (requestedIndex !== null) {
			const target = requestedIndex * carousel.clientWidth * (rtl ? -1 : 1);
			if (Math.abs(carousel.scrollLeft - target) > 1) return;
			requestedIndex = null;
		}
		carousel.style.removeProperty('scroll-snap-type');
		const index = comicSnapIndex(carousel.scrollLeft, carousel.clientWidth, groups.length);
		if (index !== activeIndex) onpagechange(groups[index][0]);
	}
	function scrollActivity() {
		if (settleTimer) clearTimeout(settleTimer);
		settleTimer = setTimeout(settled, 160);
	}
	export function prepareZoom() {
		if (!carousel) return;
		const index = comicSnapIndex(carousel.scrollLeft, carousel.clientWidth, groups.length);
		align(index, 'auto');
		if (index !== activeIndex) onpagechange(groups[index][0]);
		oncontentchange(contents[index]);
		dragStart = null;
		dragging = false;
		carousel.style.removeProperty('scroll-snap-type');
	}
	function beginGesture(event: PointerEvent) {
		if (!carousel || zoom !== 1 || event.button !== 0) return;
		// A second finger belongs to pinch zoom, never to a second page drag.
		if (!event.isPrimary) { prepareZoom(); return; }
		const pane = (event.target as Element).closest<HTMLElement>('.comic-page-pan');
		// Wide actual-size/fit-height pages pan inside their own frame first.
		const pan = pane && pane.scrollWidth > pane.clientWidth + 1 ? pane : undefined;
		if (pan && event.pointerType === 'mouse') return;
		if (settleTimer) clearTimeout(settleTimer);
		settleTimer = null;
		stopAnimation();
		const scroll = carousel.scrollLeft;
		const index = requestedIndex ?? comicSnapIndex(scroll, carousel.clientWidth, groups.length);
		requestedIndex = null;
		// Stop at the visible position, not at the old destination. Own horizontal
		// touch movement so the browser cannot resume its previous snap animation.
		carousel.style.scrollSnapType = 'none';
		carousel.scrollTo({ left: scroll, behavior: 'instant' });
		dragStart = { id: event.pointerId, x: event.clientX, y: event.clientY, scroll: pan?.scrollLeft ?? scroll, index, pan,
			lastX: event.clientX, lastTime: performance.now(), velocity: 0 };
	}
	function moveGesture(event: PointerEvent) {
		if (!dragStart || !carousel || event.pointerId !== dragStart.id) return;
		if (!dragging) {
			if (Math.abs(event.clientX - dragStart.x) <= 10 || Math.abs(event.clientX - dragStart.x) < Math.abs(event.clientY - dragStart.y)) return;
			dragging = true;
			carousel.style.scrollSnapType = 'none';
			carousel.setPointerCapture(event.pointerId);
		}
		event.preventDefault();
		const now = performance.now();
		dragStart.velocity = (event.clientX - dragStart.lastX) / Math.max(1, now - dragStart.lastTime);
		dragStart.lastX = event.clientX;
		dragStart.lastTime = now;
		const surface = dragStart.pan ?? carousel;
		surface.scrollLeft = dragStart.scroll - (event.clientX - dragStart.x);
	}
	function endGesture(event: PointerEvent) {
		if (!dragStart || event.pointerId !== dragStart.id) return;
		const start = dragStart;
		dragStart = null;
		if (!carousel) return;
		if (!dragging) {
			align(start.index, 'smooth');
			return;
		}
		if (carousel.hasPointerCapture(event.pointerId)) carousel.releasePointerCapture(event.pointerId);
		dragging = false;
		if (start.pan) { align(start.index, 'auto'); return; }
		const delta = event.clientX - start.x;
		const initial = start.index;
		let index = comicSnapIndex(carousel.scrollLeft, carousel.clientWidth, groups.length);
		if (Math.abs(delta) > carousel.clientWidth * 0.15 || (performance.now() - start.lastTime < 100 && Math.abs(start.velocity) > 0.45)) {
			index = Math.max(0, Math.min(groups.length - 1, initial + (delta < 0 ? 1 : -1) * (rtl ? -1 : 1)));
		}
		align(index, 'smooth', performance.now() - start.lastTime < 100 ? -start.velocity : 0);
	}
	function cancelGesture() {
		if (!dragStart && !dragging) return;
		dragStart = null;
		dragging = false;
		if (carousel) align(comicSnapIndex(carousel.scrollLeft, carousel.clientWidth, groups.length), 'smooth');
	}

	$effect(() => oncontentchange(contents[activeIndex]));
	$effect(() => {
		const index = activeIndex;
		const key = `${total}:${spread}:${rtl}:${width}:${height}`;
		const behavior = layoutKey !== key ? 'auto' : transition;
		layoutKey = key;
		void tick().then(() => align(index, behavior));
	});
	onDestroy(() => { stopAnimation(); if (settleTimer) clearTimeout(settleTimer); });
</script>

<svelte:window onpointermove={moveGesture} onpointerup={endGesture} onpointercancel={cancelGesture} onblur={cancelGesture} />
<div bind:this={carousel} role="region" aria-label="Comic pages" class="comic-pager" class:dragging class:zoomed={zoom !== 1}
	style:direction={rtl ? 'rtl' : 'ltr'} onpointerdown={beginGesture} onwheel={() => requestedIndex = null} onscroll={scrollActivity} onscrollend={settled}>
	{#each groups as pages, index (pages[0])}
		<div class="comic-slide" data-pages={pages.join(',')} data-active={index === activeIndex}>
			<div class="comic-page-pan" class:zoomed={index === activeIndex && zoom !== 1}>
				<div class="page-viewer" class:fit-page={fitPage} class:spread={pages.length > 1}
					bind:this={contents[index]} style:zoom={index === activeIndex ? zoom : 1}
					style:width={width ? `${width}px` : undefined} style:height={height ? `${height}px` : undefined}
					style:grid-template-columns={fitPage ? pages.map(page => `${aspectRatios[page] ?? 2 / 3}fr`).join(' ') : undefined}>
					{#each pages as page (page)}
						<img src={getPageUrl(page)} alt="Page {page}" data-page={page} draggable="false"
							class="page-image fit-{fitMode}" loading={Math.abs(index - activeIndex) <= 1 ? 'eager' : 'lazy'} decoding="async"
							onload={(event) => onimageload(page, event)} onerror={onimageerror}
							style:filter={`saturate(${saturation}%) brightness(${brightness / 100})`} />
					{/each}
				</div>
			</div>
		</div>
	{/each}
</div>

<style>
	.comic-pager { display: flex; flex: 1; width: 100%; height: 100%; min-width: 0; min-height: 0; overflow-x: auto; overflow-y: hidden; scroll-snap-type: x mandatory; scrollbar-width: none; overscroll-behavior-x: contain; touch-action: pan-y; }
	.comic-pager::-webkit-scrollbar { display: none; }
	.comic-pager.dragging, .comic-pager.zoomed { scroll-snap-type: none; }
	.comic-pager.zoomed { overflow-x: hidden; }
	.comic-slide { flex: 0 0 100%; width: 100%; height: 100%; min-width: 0; scroll-snap-align: start; scroll-snap-stop: always; overflow: hidden; }
	.comic-page-pan { touch-action: pan-y; display: flex; align-items: safe center; justify-content: safe center; width: 100%; height: 100%; overflow: auto; overscroll-behavior-y: contain; }
	.comic-page-pan.zoomed { touch-action: none; }
	.page-viewer { flex-shrink: 0; display: flex; align-items: safe center; justify-content: safe center; gap: 0; width: 100%; height: 100%; user-select: none; }
	.page-viewer.fit-page { display: grid; grid-template-rows: minmax(0, 1fr); }
	.page-viewer.fit-page .page-image { width: 100%; height: 100%; min-width: 0; min-height: 0; }
	.page-image { object-fit: contain; flex: 0 0 auto; }
	.page-image.fit-fit-page, .page-image.fit-automatic { max-width: 100%; max-height: 100%; width: auto; height: auto; }
	.page-image.fit-fit-width { width: 100%; max-width: 100%; height: auto; max-height: none; }
	.page-viewer.spread .page-image.fit-fit-width { width: 50%; }
	.page-image.fit-fit-height { height: 100%; max-height: 100%; width: auto; max-width: none; }
	.page-image.fit-actual-size { width: auto; height: auto; max-width: none; max-height: none; }
</style>
