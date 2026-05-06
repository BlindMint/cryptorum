export type LibraryIconSource = 'prime' | 'svg' | 'custom';

export interface LibraryIconDefinition {
	source: Exclude<LibraryIconSource, 'custom'>;
	name: string;
	label: string;
	svg: string;
	value: string;
}

export interface ParsedLibraryIcon {
	source: LibraryIconSource;
	name: string;
	label: string;
	svg: string;
	value: string;
}

const svgFrame = (content: string): string => `
<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
	${content}
</svg>`.trim();

const PRIME_ICON_SPECS: Array<[string, string, string]> = [
	['book', 'Book', '<path d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"/>'],
	['bookmark', 'Bookmark', '<path d="M6 3h12a1 1 0 0 1 1 1v17l-7-4-7 4V4a1 1 0 0 1 1-1z"/>'],
	['library', 'Library', '<path d="M19 11H5m14 0a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6a2 2 0 0 1 2-2m14 0V9a2 2 0 0 0-2-2M5 11V9a2 2 0 0 1 2-2m0 0V5a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v2M7 7h10"/>'],
	['folder', 'Folder', '<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/>'],
	['search', 'Search', '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>'],
	['tag', 'Tag', '<path d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 0 1 0 2.828l-7 7a2 2 0 0 1-2.828 0l-7-7A1.994 1.994 0 0 1 3 12V7a4 4 0 0 1 4-4z"/>'],
	['globe', 'Globe', '<path d="M21 12a9 9 0 0 1-9 9m9-9a9 9 0 0 0-9-9m9 9H3m9 9a9 9 0 0 1-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9"/>'],
	['star', 'Star', '<path d="M12 2.5l2.94 5.96 6.58.96-4.76 4.64 1.12 6.55L12 17.92 6.12 20.61l1.12-6.55-4.76-4.64 6.58-.96L12 2.5z"/>'],
	['heart', 'Heart', '<path d="M20.8 4.6a5.6 5.6 0 0 0-7.9 0L12 5.5l-.9-.9a5.6 5.6 0 0 0-7.9 7.9l.9.9L12 22l7.9-8.6.9-.9a5.6 5.6 0 0 0 0-7.9z"/>'],
	['cog', 'Settings', '<path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.573-1.066z"/><circle cx="12" cy="12" r="3"/>'],
	['users', 'Users', '<path d="M17 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/><path d="M20 21v-1a3 3 0 0 0-3-3"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>'],
	['trash', 'Trash', '<path d="M4 7h16"/><path d="M6 7l1 13a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-13"/><path d="M9 7V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v3"/><path d="M10 11v6"/><path d="M14 11v6"/>'],
	['sparkles', 'Sparkles', '<path d="M12 2l1.6 4.4L18 8l-4.4 1.6L12 14l-1.6-4.4L6 8l4.4-1.6L12 2z"/><path d="M19 13l.9 2.5L22 16l-2.1.5L19 19l-.9-2.5L16 16l2.1-.5L19 13z"/><path d="M5 13l.9 2.5L8 16l-2.1.5L5 19l-.9-2.5L2 16l2.1-.5L5 13z"/>'],
	['image', 'Image', '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 16-5-5-8 8"/>']
];

const SVG_LIBRARY_SPECS: Array<[string, string, string]> = [
	['rocket', 'Rocket', '<path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"/><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/>'],
	['ghost', 'Ghost', '<path d="M9 10h.01"/><path d="M15 10h.01"/><path d="M12 2a8 8 0 0 0-8 8v12l3-3 2.5 2.5L12 19l2.5 2.5L17 19l3 3V10a8 8 0 0 0-8-8z"/>'],
	['flame-kindling', 'Flame', '<path d="M12 2c1 3 2.5 3.5 3.5 4.5A5 5 0 0 1 17 10a5 5 0 1 1-10 0c0-.3 0-.6.1-.9a2 2 0 1 0 3.3-2C8 4.5 11 2 12 2Z"/><path d="m5 22 14-4"/><path d="m5 18 14 4"/>'],
	['ferris-wheel', 'Ferris Wheel', '<circle cx="12" cy="12" r="2"/><path d="M12 2v4"/><path d="m6.8 15-3.5 2"/><path d="m20.7 7-3.5 2"/><path d="M6.8 9 3.3 7"/><path d="m20.7 17-3.5-2"/><path d="m9 22 3-8 3 8"/><path d="M8 22h8"/><path d="M18 18.7a9 9 0 1 0-12 0"/>'],
	['plane', 'Plane', '<path d="M17.8 19.2 16 11l3.5-3.5C21 6 21.5 4 21 3c-1-.5-3 0-4.5 1.5L13 8 4.8 6.2c-.5-.1-.9.1-1.1.5l-.3.5c-.2.5-.1 1 .3 1.3L9 12l-2 3H4l-1 1 3 2 2 3 1-1v-3l3-2 3.5 5.3c.3.4.8.5 1.3.3l.5-.2c.4-.3.6-.7.5-1.2z"/>'],
	['skull', 'Skull', '<path d="m12.5 17-.5-1-.5 1h1z"/><path d="M15 22a1 1 0 0 0 1-1v-1a2 2 0 0 0 1.56-3.25 8 8 0 1 0-11.12 0A2 2 0 0 0 8 20v1a1 1 0 0 0 1 1z"/><circle cx="15" cy="12" r="1"/><circle cx="9" cy="12" r="1"/>'],
	['rose', 'Rose', '<path d="M17 10h-1a4 4 0 1 1 4-4v.534"/><path d="M17 6h1a4 4 0 0 1 1.42 7.74l-2.29.87a6 6 0 0 1-5.339-10.68l2.069-1.31"/><path d="M4.5 17c2.8-.5 4.4 0 5.5.8s1.8 2.2 2.3 3.7c-2 .4-3.5.4-4.8-.3-1.2-.6-2.3-1.9-3-4.2"/><path d="M9.77 12C4 15 2 22 2 22"/><circle cx="17" cy="8" r="2"/>'],
	['roller-coaster', 'Roller Coaster', '<path d="M6 19V5"/><path d="M10 19V6.8"/><path d="M14 19v-7.8"/><path d="M18 5v4"/><path d="M18 19v-6"/><path d="M22 19V9"/><path d="M2 19V9a4 4 0 0 1 4-4c2 0 4 1.33 6 4s4 4 6 4a4 4 0 1 0-3-6.65"/>'],
	['tree-palm', 'Palm Tree', '<path d="M13 8c0-2.76-2.46-5-5.5-5S2 5.24 2 8h2l1-1 1 1h4"/><path d="M13 7.14A5.82 5.82 0 0 1 16.5 6c3.04 0 5.5 2.24 5.5 5h-3l-1-1-1 1h-3"/><path d="M5.89 9.71c-2.15 2.15-2.3 5.47-.35 7.43l4.24-4.25.7-.7.71-.71 2.12-2.12c-1.95-1.96-5.27-1.8-7.42.35"/><path d="M11 15.5c.5 2.5-.17 4.5-1 6.5h4c2-5.5-.5-12-1-14"/>'],
	['tent-tree', 'Tent Tree', '<circle cx="4" cy="4" r="2"/><path d="m14 5 3-3 3 3"/><path d="m14 10 3-3 3 3"/><path d="M17 14V2"/><path d="M17 14H7l-5 8h20Z"/><path d="M8 14v8"/><path d="m9 14 5 8"/>'],
	['swords', 'Swords', '<polyline points="14.5 17.5 3 6 3 3 6 3 17.5 14.5"/><line x1="13" x2="19" y1="19" y2="13"/><line x1="16" x2="20" y1="16" y2="20"/><line x1="19" x2="21" y1="21" y2="19"/><polyline points="14.5 6.5 18 3 21 3 21 6 17.5 9.5"/><line x1="5" x2="9" y1="14" y2="18"/><line x1="7" x2="4" y1="17" y2="20"/><line x1="3" x2="5" y1="19" y2="21"/>'],
	['snail', 'Snail', '<path d="M2 13a6 6 0 1 0 12 0 4 4 0 1 0-8 0 2 2 0 0 0 4 0"/><circle cx="10" cy="13" r="8"/><path d="M2 21h12c4.4 0 8-3.6 8-8V7a2 2 0 1 0-4 0v6"/><path d="M18 3 19.1 5.2"/><path d="M22 3 20.9 5.2"/>'],
	['drama', 'Drama', '<path d="M10 11h.01"/><path d="M14 6h.01"/><path d="M18 6h.01"/><path d="M6.5 13.1h.01"/><path d="M22 5c0 9-4 12-6 12s-6-3-6-12c0-2 2-3 6-3s6 1 6 3"/><path d="M17.4 9.9c-.8.8-2 .8-2.8 0"/><path d="M10.1 7.1C9 7.2 7.7 7.7 6 8.6c-3.5 2-4.7 3.9-3.7 5.6 4.5 7.8 9.5 8.4 11.2 7.4.9-.5 1.9-2.1 1.9-4.7"/><path d="M9.1 16.5c.3-1.1 1.4-1.7 2.4-1.4"/>'],
	['chef-hat', 'Chef Hat', '<path d="M17 21a1 1 0 0 0 1-1v-5.35c0-.457.316-.844.727-1.041a4 4 0 0 0-2.134-7.589 5 5 0 0 0-9.186 0 4 4 0 0 0-2.134 7.588c.411.198.727.585.727 1.041V20a1 1 0 0 0 1 1Z"/><path d="M6 17h12"/>']
];

function makeIconDefinition(source: Exclude<LibraryIconSource, 'custom'>, name: string, label: string, svg: string): LibraryIconDefinition {
	return {
		source,
		name,
		label,
		svg: svgFrame(svg),
		value: `${source}|${name}`
	};
}

export const PRIME_LIBRARY_ICONS: LibraryIconDefinition[] = PRIME_ICON_SPECS.map(([name, label, svg]) =>
	makeIconDefinition('prime', name, label, svg)
);

export const SVG_LIBRARY_ICONS: LibraryIconDefinition[] = SVG_LIBRARY_SPECS.map(([name, label, svg]) =>
	makeIconDefinition('svg', name, label, svg)
);

export function sanitizeSvgMarkup(svg: string): string {
	return svg
		.replace(/<script[\s\S]*?<\/script>/gi, '')
		.replace(/<foreignObject[\s\S]*?<\/foreignObject>/gi, '')
		.replace(/<style[\s\S]*?<\/style>/gi, '')
		.trim();
}

export function serializeCustomLibraryIcon(name: string, svg: string): string {
	return `custom|${encodeURIComponent(name.trim())}|${encodeURIComponent(svg.trim())}`;
}

export function parseLibraryIcon(value: string | null | undefined): ParsedLibraryIcon | null {
	if (!value || !value.trim()) return null;

	const trimmed = value.trim();
	const customMatch = /^custom\|([^|]+)\|(.+)$/.exec(trimmed);
	if (customMatch) {
		const name = decodeURIComponent(customMatch[1]);
		const svg = decodeURIComponent(customMatch[2]);
		return {
			source: 'custom',
			name,
			label: name,
			svg,
			value: trimmed
		};
	}

	const normalized = /^([a-z]+)\|(.+)$/.exec(trimmed);
	if (normalized) {
		const source = normalized[1] as LibraryIconSource;
		const name = normalized[2];
		if (source === 'prime') {
			const icon = PRIME_LIBRARY_ICONS.find(item => item.name === name);
			return icon ? { ...icon } : { source: 'prime', name, label: name, svg: '', value: trimmed };
		}
		if (source === 'svg') {
			const icon = SVG_LIBRARY_ICONS.find(item => item.name === name);
			return icon ? { ...icon } : { source: 'svg', name, label: name, svg: '', value: trimmed };
		}
	}

	const prime = PRIME_LIBRARY_ICONS.find(item => item.name === trimmed);
	if (prime) return { ...prime };

	return {
		source: 'prime',
		name: trimmed,
		label: trimmed,
		svg: '',
		value: `prime|${trimmed}`
	};
}

export function iconButtonLabel(icon: string | null | undefined): string {
	const parsed = parseLibraryIcon(icon);
	if (!parsed) return '';
	if (parsed.source === 'custom') return `Custom SVG: ${parsed.name}`;
	return `${parsed.source === 'svg' ? 'SVG' : 'Prime'}: ${parsed.name}`;
}
