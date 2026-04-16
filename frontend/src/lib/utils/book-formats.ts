const AUDIO_FORMATS = new Set(['mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav', 'aac', 'opus']);
const PDF_FORMATS = new Set(['pdf']);
const CBX_FORMATS = new Set(['cbz', 'cbr', 'cb7', 'cbt']);
const TEXT_FORMATS = new Set(['epub', 'mobi', 'azw', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf']);
const FORMAT_PRIORITY = ['epub', 'pdf', 'cbz', 'cbr', 'cb7', 'cbt', 'mobi', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf', 'mp3', 'm4b', 'm4a', 'flac', 'ogg', 'wav', 'aac', 'opus'];
const TEXT_PRIORITY = ['epub', 'mobi', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf'];

export type ReaderRouteKind = 'epub' | 'pdf' | 'cbx' | 'audio' | null;

export type BookFile = {
	format?: string;
};

export function normalizeBookFormat(format: string | null | undefined): string {
	return String(format || '').trim().toLowerCase();
}

export function getReaderRouteKind(format: string | null | undefined): ReaderRouteKind {
	const normalized = normalizeBookFormat(format);
	if (!normalized) return null;
	if (AUDIO_FORMATS.has(normalized)) return 'audio';
	if (PDF_FORMATS.has(normalized)) return 'pdf';
	if (CBX_FORMATS.has(normalized)) return 'cbx';
	if (TEXT_FORMATS.has(normalized)) return 'epub';
	return null;
}

export function getBookReaderHref(bookId: number | string, format: string | null | undefined): string {
	const normalized = normalizeBookFormat(format);
	const routeKind = getReaderRouteKind(normalized);

	switch (routeKind) {
		case 'audio':
			return `/reader/audio/${bookId}?format=${encodeURIComponent(normalized)}`;
		case 'pdf':
			return `/reader/pdf/${bookId}?format=${encodeURIComponent(normalized)}`;
		case 'cbx':
			return `/reader/cbx/${bookId}?format=${encodeURIComponent(normalized)}`;
		case 'epub':
			return `/reader/epub/${bookId}?format=${encodeURIComponent(normalized)}`;
		default:
			return `/reader/epub/${bookId}`;
	}
}

export function getSpeedReaderHref(bookId: number | string, format: string | null | undefined): string {
	const normalized = normalizeBookFormat(format);
	if (normalized) {
		return `/reader/speed/${bookId}?format=${encodeURIComponent(normalized)}`;
	}
	return `/reader/speed/${bookId}`;
}

export function getFormatDisplayLabel(format: string | null | undefined): string {
	const normalized = normalizeBookFormat(format);
	return normalized ? normalized.toUpperCase() : 'UNKNOWN';
}

export function uniqueBookFormats(files: BookFile[]): string[] {
	const seen = new Set<string>();
	const formats: string[] = [];
	for (const file of files) {
		const normalized = normalizeBookFormat(file?.format);
		if (!normalized || seen.has(normalized)) continue;
		seen.add(normalized);
		formats.push(normalized);
	}
	return formats;
}

export function getPreferredBookFormat(files: BookFile[]): string | null {
	const formats = uniqueBookFormats(files);
	if (formats.length === 0) return null;
	for (const format of FORMAT_PRIORITY) {
		if (formats.includes(format)) {
			return format;
		}
	}
	return formats[0];
}

export function getPreferredTextFormat(files: BookFile[]): string | null {
	const formats = uniqueBookFormats(files).filter((format) => getReaderRouteKind(format) === 'epub');
	if (formats.length === 0) return null;
	for (const format of TEXT_PRIORITY) {
		if (formats.includes(format)) {
			return format;
		}
	}
	return formats[0];
}
