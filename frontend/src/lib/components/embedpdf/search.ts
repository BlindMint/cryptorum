import { embedPdfSelectors } from './constants';
import { isVisibleTextInput, queryAllDeep } from './dom';

export function getEmbedPdfSearchInput(root: ParentNode | null) {
	if (!root) return null;
	return queryAllDeep(embedPdfSelectors.searchInput, root).find(isVisibleTextInput) ?? null;
}

export function getDeepActiveElement(root: Document | ShadowRoot = document): Element | null {
	let activeElement = root.activeElement;
	while (activeElement instanceof HTMLElement && activeElement.shadowRoot?.activeElement) {
		activeElement = activeElement.shadowRoot.activeElement;
	}
	return activeElement;
}

export function focusEmbedPdfSearchInput(root: ParentNode | null, selectText = true) {
	const input = getEmbedPdfSearchInput(root);
	if (!input) return false;

	input.focus({ preventScroll: true });
	if (selectText) {
		input.select();
	}

	return getDeepActiveElement() === input;
}
