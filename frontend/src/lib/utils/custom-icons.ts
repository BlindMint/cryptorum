export function defaultSvgThemeColor(svg: string): boolean {
	return !/(?:\sfill\s*=|(?:^|[;"'])\s*fill\s*:)/i.test(svg);
}

// Keep transparent cutouts and stroke-only artwork; don't introduce outlines
// on filled icons. Only change paint declarations, never path geometry.
export function themeSvgMarkup(svg: string): string {
	return svg.replace(/<[a-z][a-z0-9:-]*(?:\s+(?:[^<>"']|"[^"]*"|'[^']*')*)?\/?>/gi, tag => {
		tag = tag.replace(/(\s)(fill|stroke|color)\s*=\s*(?:"([^"]*)"|'([^']*)')/gi, (match, space, property, double, single) => {
			const value = (double ?? single).trim().toLowerCase();
			if (property.toLowerCase() !== 'color' && (value === 'none' || value === 'transparent')) return match;
			return `${space}${property}="${property.toLowerCase() === 'color' ? 'inherit' : 'currentColor'}"`;
		});
		tag = tag.replace(/(\sstyle\s*=\s*)(["'])(.*?)\2/gi, (_match, prefix, quote, style) => {
			const themed = style.replace(/(^|;)\s*(fill|stroke|color)\s*:\s*([^;]+)/gi, (declaration: string, separator: string, property: string, value: string) => {
				const paint = value.replace(/\s*!important\s*$/i, '').trim().toLowerCase();
				if (property.toLowerCase() !== 'color' && (paint === 'none' || paint === 'transparent')) return declaration;
				return `${separator}${property}:${property.toLowerCase() === 'color' ? 'inherit' : 'currentColor'}${/!important/i.test(value) ? ' !important' : ''}`;
			});
			return prefix + quote + themed + quote;
		});
		if (/^<svg\b/i.test(tag) && !/\sfill\s*=/i.test(tag)) tag = tag.replace(/<svg\b/i, '<svg fill="currentColor"');
		return tag;
	});
}

export function isPngIconDataUrl(value: string): boolean {
	return value.length <= 1024 * 1024 && /^data:image\/png;base64,iVBORw0KGgo[A-Za-z0-9+/]*={0,2}$/.test(value);
}

export async function readPngIcon(file: File): Promise<string> {
	if (file.size > 5 * 1024 * 1024) throw new Error('Choose a PNG smaller than 5 MB.');
	const bytes = new Uint8Array(await file.slice(0, 8).arrayBuffer());
	if (![137, 80, 78, 71, 13, 10, 26, 10].every((byte, index) => bytes[index] === byte)) {
		throw new Error('Choose a valid PNG image.');
	}
	const url = URL.createObjectURL(file);
	try {
		const image = new Image();
		image.src = url;
		await image.decode();
		const scale = Math.min(1, 256 / image.naturalWidth, 256 / image.naturalHeight);
		const canvas = document.createElement('canvas');
		canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
		canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
		const context = canvas.getContext('2d');
		if (!context) throw new Error('Could not prepare this PNG image.');
		context.drawImage(image, 0, 0, canvas.width, canvas.height);
		return canvas.toDataURL('image/png');
	} finally { URL.revokeObjectURL(url); }
}
