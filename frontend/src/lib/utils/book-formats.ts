export const SUPPORTED_AUDIO_FORMATS = ['mp3', 'm4a', 'm4b', 'flac', 'ogg', 'wav'] as const;

const AUDIO_FORMATS = new Set<string>(SUPPORTED_AUDIO_FORMATS);
const PDF_FORMATS = new Set(['pdf']);
const CBX_FORMATS = new Set(['cbz', 'cbr', 'cb7', 'cbt']);
const TEXT_FORMATS = new Set(['epub', 'mobi', 'azw', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf']);
const FORMAT_PRIORITY = ['epub', 'pdf', 'cbz', 'cbr', 'cb7', 'cbt', 'mobi', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf', 'mp3', 'm4b', 'm4a', 'flac', 'ogg', 'wav'];
const TEXT_PRIORITY = ['epub', 'mobi', 'azw3', 'azw4', 'fb2', 'docx', 'html', 'rtf', 'txt', 'text', 'odt', 'pdb', 'lrf'];

export type ReaderRouteKind = 'epub' | 'pdf' | 'cbx' | 'audio' | null;
export type BookMediaKind = 'book' | 'audio';

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

export function isAudioFormat(format: string | null | undefined): boolean {
	return AUDIO_FORMATS.has(normalizeBookFormat(format));
}

export function getBookMediaKind(format: string | null | undefined): BookMediaKind {
	return isAudioFormat(format) ? 'audio' : 'book';
}

function appendReturnTo(href: string, returnTo?: string | null): string {
	if (!returnTo) return href;
	const separator = href.includes('?') ? '&' : '?';
	return `${href}${separator}returnTo=${encodeURIComponent(returnTo)}`;
}

export function getBookReaderHref(
	bookId: number | string,
	format: string | null | undefined,
	returnTo?: string | null
): string {
	const normalized = normalizeBookFormat(format);
	const routeKind = getReaderRouteKind(normalized);
	let href: string;

	switch (routeKind) {
		case 'audio':
			href = `/reader/audio/${bookId}?format=${encodeURIComponent(normalized)}`;
			break;
		case 'pdf':
			href = `/reader/pdf/${bookId}?format=${encodeURIComponent(normalized)}`;
			break;
		case 'cbx':
			href = `/reader/cbx/${bookId}?format=${encodeURIComponent(normalized)}`;
			break;
		case 'epub':
			href = `/reader/epub/${bookId}?format=${encodeURIComponent(normalized)}`;
			break;
		default:
			href = `/reader/epub/${bookId}`;
	}
	return appendReturnTo(href, returnTo);
}

export function getSpeedReaderHref(
	bookId: number | string,
	format: string | null | undefined,
	returnTo?: string | null
): string {
	const normalized = normalizeBookFormat(format);
	let href: string;
	if (normalized) {
		href = `/reader/speed/${bookId}?format=${encodeURIComponent(normalized)}`;
	} else {
		href = `/reader/speed/${bookId}`;
	}
	return appendReturnTo(href, returnTo);
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
