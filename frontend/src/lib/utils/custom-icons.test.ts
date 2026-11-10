import { describe, expect, it } from 'vitest';
import { defaultSvgThemeColor, themeSvgMarkup, isPngIconDataUrl } from './custom-icons';
import { parseLibraryIcon, serializeCustomLibraryIcon, serializePngLibraryIcon, iconButtonLabel } from './library-icons';

const png = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';

describe('custom SVG theme colors', () => {
	it('uses theme fill automatically when legacy artwork has no explicit fill', () => {
		const raw = '<svg viewBox="0 0 24 24"><path d="M0 0h24v24z"/></svg>';
		const parsed = parseLibraryIcon(serializeCustomLibraryIcon('Ninja', raw));
		expect(parsed?.svg).toContain('fill="currentColor"');
		expect(parsed?.useThemeColor).toBe(true);
		expect(parsed?.rawSvg).toBe(raw);
		expect(defaultSvgThemeColor('<svg style="fill: red"></svg>')).toBe(false);
		expect(defaultSvgThemeColor('<svg fill="none"></svg>')).toBe(false);
	});
	it('themes fills and strokes while retaining transparent cutouts, opacity and geometry', () => {
		const raw = '<svg fill="#000" color="red"><path fill="none" stroke="black" opacity=".5" fill-rule="evenodd" d="M1 2h3"/><path fill="transparent" stroke="none"/></svg>';
		const themed = themeSvgMarkup(raw);
		expect(themed).toContain('fill="currentColor"');
		expect(themed).toContain('color="inherit"');
		expect(themed).toContain('fill="none" stroke="currentColor"');
		expect(themed).toContain('opacity=".5" fill-rule="evenodd" d="M1 2h3"');
		expect(themed).toContain('fill="transparent" stroke="none"');
	});
	it('handles inline styles without adding strokes to filled icons', () => {
		const themed = themeSvgMarkup('<svg style="fill: #000 !important; color:red"><path style="fill:none;stroke:blue;stroke-width:2"/></svg>');
		expect(themed).toContain('fill:currentColor !important');
		expect(themed).toContain('color:inherit');
		expect(themed).toContain('fill:none;stroke:currentColor;stroke-width:2');
		expect(themeSvgMarkup('<svg><path fill="black"/></svg>')).not.toContain('stroke=');
	});
	it('remembers the toggle and restores original colors without altering the saved SVG', () => {
		const raw = '<svg fill="#123456"><path fill="#abcdef"/></svg>';
		const theme = parseLibraryIcon(serializeCustomLibraryIcon('Ninja', raw, true));
		expect(theme?.svg).not.toContain('#123456');
		expect(theme?.rawSvg).toBe(raw);
		const original = parseLibraryIcon(serializeCustomLibraryIcon('Ninja', theme!.rawSvg!, false));
		expect(original?.svg).toBe(raw);
		expect(original?.useThemeColor).toBe(false);
		expect(parseLibraryIcon(serializeCustomLibraryIcon('Colored', raw))?.useThemeColor).toBe(false);
	});
});

describe('PNG library icons', () => {
	it('round trips a PNG as a self-contained icon using the shared SVG renderer', () => {
		const value = serializePngLibraryIcon('Ninja image', png);
		const icon = parseLibraryIcon(value);
		expect(icon?.source).toBe('image');
		expect(icon?.imageDataUrl).toBe(png);
		expect(icon?.svg).toContain('<image href="' + png + '"');
		expect(icon?.svg).toContain('preserveAspectRatio="xMidYMid meet"');
		expect(iconButtonLabel(value)).toBe('PNG: Ninja image');
	});
	it('rejects external URLs, other image types and malformed encoded values', () => {
		expect(isPngIconDataUrl('https://example.com/icon.png')).toBe(false);
		expect(isPngIconDataUrl('data:image/svg+xml;base64,AAAA')).toBe(false);
		expect(isPngIconDataUrl(png + '" onload="bad')).toBe(false);
		expect(() => serializePngLibraryIcon('Bad', 'not an image')).toThrow();
		expect(parseLibraryIcon('image|Bad|data:image/svg+xml;base64,AAAA')).toBeNull();
		expect(parseLibraryIcon('custom|%invalid|%invalid')).toBeNull();
	});
});
