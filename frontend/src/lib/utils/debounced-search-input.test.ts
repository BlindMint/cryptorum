import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createDebouncedSearchInput } from './debounced-search-input';

beforeEach(() => {
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe('createDebouncedSearchInput', () => {
	it('commits an ordinary input after the normal quiet period', () => {
		const commit = vi.fn();
		const controller = createDebouncedSearchInput({
			debounceMs: 650,
			compositionFallbackMs: 1000,
			onCommit: commit
		});

		controller.input('gaunts ghosts');
		vi.advanceTimersByTime(649);
		expect(commit).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(commit).toHaveBeenCalledWith('gaunts ghosts');
	});

	it('falls back when a mobile keyboard never ends composition', () => {
		const commit = vi.fn();
		const controller = createDebouncedSearchInput({
			debounceMs: 650,
			compositionFallbackMs: 1000,
			onCommit: commit
		});

		controller.compositionStart();
		controller.input('gaunts', true);
		vi.advanceTimersByTime(999);
		expect(commit).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(commit).toHaveBeenCalledWith('gaunts');
	});

	it('commits Enter immediately and cancels the delayed duplicate', () => {
		const commit = vi.fn();
		const controller = createDebouncedSearchInput({
			debounceMs: 650,
			compositionFallbackMs: 1000,
			onCommit: commit
		});

		controller.compositionStart();
		controller.input('sabbat worlds crusade', true);
		controller.submit('sabbat worlds crusade');
		expect(commit).toHaveBeenCalledTimes(1);
		expect(commit).toHaveBeenCalledWith('sabbat worlds crusade');
		vi.runAllTimers();
		expect(commit).toHaveBeenCalledTimes(1);
	});
});
