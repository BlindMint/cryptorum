import { isScrollableElement, queryAllDeep } from './dom';

export type ScrollActivityCallback = (delta: number, scrollTop: number) => void;

export function createEmbedPdfScrollActivityController(onScrollActivity?: ScrollActivityCallback) {
	let scrollListenerTargets = new Set<HTMLElement>();
	let scrollPositions = new WeakMap<HTMLElement, number>();
	let scrollActivityFrame: number | null = null;
	let pendingScrollActivity: { delta: number; scrollTop: number } | null = null;

	function handleInternalScroll(event: Event) {
		const target = event.currentTarget;
		if (!(target instanceof HTMLElement)) return;

		const scrollTop = target.scrollTop;
		const previousScrollTop = scrollPositions.get(target) ?? scrollTop;
		const delta = scrollTop - previousScrollTop;
		scrollPositions.set(target, scrollTop);

		if (Math.abs(delta) > 0.5) {
			pendingScrollActivity = { delta, scrollTop };
			if (scrollActivityFrame === null) {
				scrollActivityFrame = requestAnimationFrame(() => {
					scrollActivityFrame = null;
					if (!pendingScrollActivity) return;
					const activity = pendingScrollActivity;
					pendingScrollActivity = null;
					onScrollActivity?.(activity.delta, activity.scrollTop);
				});
			}
		}
	}

	function update(root: HTMLElement | null) {
		if (!root) return;

		const scrollableElements = queryAllDeep('*', root).filter((element): element is HTMLElement =>
			element instanceof HTMLElement && isScrollableElement(element)
		);

		for (const target of Array.from(scrollListenerTargets)) {
			if (!scrollableElements.includes(target)) {
				target.removeEventListener('scroll', handleInternalScroll);
				scrollListenerTargets.delete(target);
			}
		}

		for (const target of scrollableElements) {
			if (scrollListenerTargets.has(target)) continue;
			scrollListenerTargets.add(target);
			scrollPositions.set(target, target.scrollTop);
			target.addEventListener('scroll', handleInternalScroll, { passive: true });
		}
	}

	function detach() {
		for (const target of scrollListenerTargets) {
			target.removeEventListener('scroll', handleInternalScroll);
		}
		scrollListenerTargets.clear();
		scrollPositions = new WeakMap<HTMLElement, number>();
		if (scrollActivityFrame !== null) {
			cancelAnimationFrame(scrollActivityFrame);
			scrollActivityFrame = null;
		}
		pendingScrollActivity = null;
	}

	return {
		update,
		detach,
		get targetCount() {
			return scrollListenerTargets.size;
		}
	};
}
