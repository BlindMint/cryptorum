import { describe, expect, it } from 'vitest';
import { comicGlide } from './comic-motion';

describe('comic release glide', () => {
	it.each([[75, 390, 1], [-75, -390, -1], [280, 0, -1]])('decelerates continuously from %s to %s', (from, to, velocity) => {
		const glide = comicGlide(from, to, velocity);
		let previousStep = Infinity;
		let previous = from;
		for (let frame = 1; frame <= 60; frame++) {
			const position = glide.position(glide.duration * frame / 60);
			const step = (position - previous) * Math.sign(to - from);
			expect(step).toBeGreaterThanOrEqual(0);
			expect(step).toBeLessThanOrEqual(previousStep + 1e-8);
			previousStep = step;
			previous = position;
		}
		expect(previous).toBe(to);
		expect(glide.position(glide.duration + 100)).toBe(to);
	});
	it('uses release speed to shorten a fast flick without a long tail', () => {
		expect(comicGlide(100, 390, 4).duration).toBeLessThan(comicGlide(100, 390, 1).duration);
		expect(comicGlide(389, 390, 0).duration).toBeGreaterThan(0);
	});
});
