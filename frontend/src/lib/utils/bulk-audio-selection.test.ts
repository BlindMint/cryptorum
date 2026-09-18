import { describe, expect, it } from 'vitest';
import { selectionMayContainAudio } from './bulk-audio-selection';

describe('selectionMayContainAudio', () => {
	it('hides the action when every selected book is known to have no audio', () => {
		expect(selectionMayContainAudio(new Set([1, 2]), [
			{ id: 1, has_audio: false },
			{ id: 2, has_audio: false }
		])).toBe(false);
	});

	it('shows the action when any selected book has audio', () => {
		expect(selectionMayContainAudio(new Set([1, 2]), [
			{ id: 1, has_audio: false },
			{ id: 2, has_audio: true }
		])).toBe(true);
	});

	it('keeps the action available when a restored selection is not fully loaded', () => {
		expect(selectionMayContainAudio(new Set([1, 2]), [
			{ id: 1, has_audio: false }
		])).toBe(true);
	});

	it('keeps compatibility with payloads that do not yet include audio availability', () => {
		expect(selectionMayContainAudio(new Set([1]), [{ id: 1 }])).toBe(true);
	});

	it('hides the action for an empty selection', () => {
		expect(selectionMayContainAudio([], [{ id: 1, has_audio: true }])).toBe(false);
	});
});
