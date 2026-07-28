export interface DebouncedSearchInputOptions {
	debounceMs: number;
	compositionFallbackMs: number;
	onCommit: (value: string) => void;
}

export interface DebouncedSearchInputController {
	input: (value: string, isComposing?: boolean) => void;
	compositionStart: () => void;
	compositionEnd: (value: string) => void;
	submit: (value: string) => void;
	cancel: () => void;
}

/**
 * Keeps mobile predictive keyboards and full IMEs from blocking search forever.
 * Composition receives a longer quiet period, but still commits if a keyboard
 * does not emit compositionend until the user types a word separator.
 */
export function createDebouncedSearchInput(
	options: DebouncedSearchInputOptions
): DebouncedSearchInputController {
	let composing = false;
	let timer: ReturnType<typeof setTimeout> | null = null;

	function cancel() {
		if (timer) clearTimeout(timer);
		timer = null;
	}

	function schedule(value: string, delay: number) {
		cancel();
		timer = setTimeout(() => {
			timer = null;
			options.onCommit(value);
		}, delay);
	}

	return {
		input(value, isComposing = false) {
			if (isComposing) composing = true;
			schedule(value, composing ? options.compositionFallbackMs : options.debounceMs);
		},
		compositionStart() {
			composing = true;
			cancel();
		},
		compositionEnd(value) {
			composing = false;
			schedule(value, options.debounceMs);
		},
		submit(value) {
			composing = false;
			cancel();
			options.onCommit(value);
		},
		cancel
	};
}
