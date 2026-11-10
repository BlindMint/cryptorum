import sprite from '../../../static/icons/tabler/outline.svg?raw';
import { describe, expect, it } from 'vitest';
import { TABLER_LIBRARY_ICONS, parseLibraryIcon, iconButtonLabel, serializeCustomLibraryIcon } from './library-icons';

describe('Tabler library icons', () => {
	it('matches the complete local outline sprite and preserves currentColor', () => {
		const symbols = new Set([...sprite.matchAll(/<symbol id="([^"]+)"/g)].map(match => match[1]));
		expect(TABLER_LIBRARY_ICONS.length).toBe(5233);
		expect(symbols.size).toBe(TABLER_LIBRARY_ICONS.length);
		for (const icon of TABLER_LIBRARY_ICONS) {
			expect(symbols.has(icon.name)).toBe(true);
			expect(icon.svg).toContain(`/icons/tabler/outline.svg#${icon.name}`);
		}
		expect(sprite).toContain('stroke="currentColor"');
	});

	it('restores compact saved values and describes their source', () => {
		const icon = parseLibraryIcon('tabler|books');
		expect(icon?.source).toBe('tabler');
		expect(icon?.label).toBe('Books');
		expect(icon?.svg).toContain('<use href="/icons/tabler/outline.svg#books"');
		expect(iconButtonLabel('tabler|books')).toBe('Tabler: books');
		expect(parseLibraryIcon('tabler|unknown-or-removed')?.svg).toBe('');
		expect(parseLibraryIcon('tabler|"><script>bad</script>')?.svg).toBe('');
	});

	it('keeps legacy and custom icons working', () => {
		expect(parseLibraryIcon('book')?.value).toBe('prime|book');
		expect(parseLibraryIcon('prime|book')?.svg).toContain('<svg');
		expect(parseLibraryIcon('svg|rocket')?.svg).toContain('<svg');
		const value = serializeCustomLibraryIcon('My icon', '<svg viewBox="0 0 24 24"></svg>');
		expect(parseLibraryIcon(value)?.name).toBe('My icon');
		expect(parseLibraryIcon(value)?.source).toBe('custom');
	});
});
