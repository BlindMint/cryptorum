export function queryAllDeep(selector: string, root: ParentNode): Element[] {
	const matches = Array.from(root.querySelectorAll(selector));
	const children = Array.from(root.querySelectorAll('*'));

	for (const child of children) {
		const shadowRoot = (child as HTMLElement).shadowRoot;
		if (shadowRoot) {
			matches.push(...queryAllDeep(selector, shadowRoot));
		}
	}

	return matches;
}

export function queryFirstDeep(selector: string, root: ParentNode): HTMLElement | null {
	const element = queryAllDeep(selector, root)[0];
	return element instanceof HTMLElement ? element : null;
}

export function eventPathIncludesSelector(event: Event, selector: string) {
	const path = event.composedPath?.() ?? [];
	return path.some((target) => {
		if (!(target instanceof Element)) return false;
		return target.matches(selector) || !!target.closest(selector);
	});
}

export function isVisibleTextInput(element: Element): element is HTMLInputElement {
	if (!(element instanceof HTMLInputElement) || element.disabled || element.readOnly) {
		return false;
	}

	const rect = element.getBoundingClientRect();
	if (rect.width <= 0 || rect.height <= 0) return false;

	const style = getComputedStyle(element);
	return style.display !== 'none' && style.visibility !== 'hidden';
}

export function setStyle(
	element: HTMLElement,
	property: string,
	value: string,
	priority: '' | 'important' = ''
) {
	if (
		element.style.getPropertyValue(property) !== value ||
		element.style.getPropertyPriority(property) !== priority
	) {
		element.style.setProperty(property, value, priority);
	}
}

export function removeStyle(element: HTMLElement, property: string) {
	if (element.style.getPropertyValue(property)) {
		element.style.removeProperty(property);
	}
}

export function isScrollableElement(element: HTMLElement) {
	const style = getComputedStyle(element);
	const verticalOverflow = style.overflowY;
	const horizontalOverflow = style.overflowX;
	const canScrollVertically =
		element.scrollHeight > element.clientHeight + 8 &&
		['auto', 'scroll', 'overlay'].includes(verticalOverflow);
	const canScrollHorizontally =
		element.scrollWidth > element.clientWidth + 8 &&
		['auto', 'scroll', 'overlay'].includes(horizontalOverflow);

	return canScrollVertically || canScrollHorizontally;
}
