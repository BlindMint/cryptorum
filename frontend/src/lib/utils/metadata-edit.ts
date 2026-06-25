export interface MetadataEditForm {
	title: string;
	series: string;
	series_number: string;
	publisher: string;
	pub_date: string;
	description: string;
	rating: number;
	status: string;
	genres: string;
	tags: string;
	isbn: string;
	asin: string;
	language: string;
	page_count: number;
	comic_spread_fallback: string;
}

export const metadataStatusOptions = [
	{ value: 'unread', label: 'Unread' },
	{ value: 'reading', label: 'Currently Reading' },
	{ value: 'finished', label: 'Already Read' }
];

export const ratingStars = [1, 2, 3, 4, 5];

export function ratingToStars(value: number | null | undefined): number {
	const rating = Number(value || 0);
	if (rating <= 0) return 0;
	return Math.max(1, Math.min(5, Math.round(rating / 2)));
}

export function starsToRating(value: number | null | undefined): number {
	const stars = Number(value || 0);
	if (stars <= 0) return 0;
	return Math.max(0, Math.min(10, Math.round(stars) * 2));
}

export const comicSpreadFallbackOptions = [
	{ value: 'inherit', label: 'Inherit' },
	{ value: 'right', label: 'Right side' },
	{ value: 'left', label: 'Left side' },
	{ value: 'disabled', label: 'Disabled' }
];

export function parseAuthors(authorsJson: string): string[] {
	try {
		const arr = JSON.parse(authorsJson);
		return Array.isArray(arr) ? arr : [authorsJson];
	} catch {
		return [authorsJson];
	}
}

export function prepareAuthorRows(authors: string[]): string[] {
	if (authors.length === 0) return [''];
	if (!authors[authors.length - 1].trim()) return authors;
	return [...authors, ''];
}

export function setAuthorRowValue(authors: string[], index: number, value: string): string[] {
	const rows = [...authors];
	rows[index] = value;
	return prepareAuthorRows(rows);
}

export function removeAuthorRow(authors: string[], index: number): string[] {
	return prepareAuthorRows(authors.filter((_, i) => i !== index));
}

export function parseJsonArray(jsonStr: string): string[] {
	if (!jsonStr || jsonStr === '[]' || jsonStr === '') return [];
	try {
		const arr = JSON.parse(jsonStr);
		return Array.isArray(arr) ? arr : [];
	} catch {
		return [];
	}
}

export function formatSeriesNumber(item: any): string {
	if (item?.series_number_display) return item.series_number_display;
	const value = Number(item?.series_number || 0);
	if (!value) return '';
	return Number.isInteger(value) ? String(value) : String(value);
}

export function createMetadataEditForm(book: any): MetadataEditForm {
	return {
		title: book?.title || '',
		series: book?.series || '',
		series_number: formatSeriesNumber(book),
		publisher: book?.publisher || '',
		pub_date: book?.pub_date || '',
		description: book?.description || '',
		rating: ratingToStars(book?.rating),
		status: book?.status || 'unread',
		genres: parseJsonArray(book?.genres || '[]').join(', '),
		tags: parseJsonArray(book?.tags || '[]').join(', '),
		isbn: book?.isbn || '',
		asin: book?.asin || '',
		language: book?.language || '',
		page_count: book?.page_count || 0,
		comic_spread_fallback: book?.comic_spread_fallback || 'inherit'
	};
}

export function normalizeAuthorName(name: string): string {
	if (!name) return name;

	name = name.trim().replace(/\s+/g, ' ');
	const { namePart, suffixes } = splitAuthorCredentialSuffixes(name);
	const normalizedName = normalizeInvertedAuthorName(namePart);
	const formattedName = formatAuthorNamePart(normalizedName);
	if (suffixes.length === 0) return formattedName;
	return `${formattedName}, ${suffixes.join(', ')}`;
}

const authorCredentialSuffixes: Record<string, string> = {
	phd: 'Ph.D.',
	md: 'M.D.',
	do: 'D.O.',
	jd: 'J.D.',
	dds: 'D.D.S.',
	dmd: 'D.M.D.',
	dvm: 'D.V.M.',
	edd: 'Ed.D.',
	psyd: 'Psy.D.',
	scd: 'Sc.D.',
	dphil: 'DPhil',
	dmin: 'D.Min.',
	thd: 'Th.D.',
	rn: 'RN',
	np: 'NP',
	pa: 'PA',
	cpa: 'CPA',
	pe: 'P.E.',
	mba: 'MBA',
	ma: 'M.A.',
	ms: 'M.S.',
	mfa: 'M.F.A.',
	mph: 'M.P.H.',
	lcsw: 'LCSW',
	lpc: 'LPC',
	lmft: 'LMFT'
};

const authorCredentialSuffixesAllowLowercase = new Set([
	'phd',
	'edd',
	'psyd',
	'scd',
	'dphil',
	'dmin',
	'thd',
	'mba',
	'mfa',
	'mph',
	'lcsw',
	'lpc',
	'lmft'
]);

const authorTitlePrefixes: Record<string, string> = {
	dr: 'Dr.',
	doctor: 'Dr.',
	prof: 'Prof.',
	professor: 'Prof.',
	rev: 'Rev.',
	reverend: 'Rev.',
	fr: 'Fr.',
	father: 'Fr.',
	sr: 'Sr.',
	sister: 'Sr.',
	br: 'Br.',
	brother: 'Br.',
	rabbi: 'Rabbi',
	imam: 'Imam',
	sheikh: 'Sheikh',
	sir: 'Sir',
	dame: 'Dame'
};

function splitAuthorCredentialSuffixes(author: string): { namePart: string; suffixes: string[] } {
	const commaParts = splitAuthorCommaParts(author);
	if (commaParts.length > 1) {
		const suffixes: string[] = [];
		const remaining = [...commaParts];
		while (remaining.length > 1) {
			const suffix = canonicalAuthorCredential(remaining[remaining.length - 1]);
			if (!suffix) break;
			suffixes.unshift(suffix);
			remaining.pop();
		}
		if (suffixes.length > 0) {
			return { namePart: remaining.join(', '), suffixes };
		}
		return { namePart: author, suffixes: [] };
	}

	const fields = author.split(/\s+/).filter(Boolean);
	const suffixes: string[] = [];
	while (fields.length > 0) {
		const result = trailingAuthorCredential(fields);
		if (!result) break;
		suffixes.unshift(result.suffix);
		fields.splice(result.index);
	}
	return { namePart: fields.join(' '), suffixes };
}

function splitAuthorCommaParts(author: string): string[] {
	return author.split(',').map((part) => part.trim()).filter(Boolean);
}

function normalizeInvertedAuthorName(author: string): string {
	const parts = splitAuthorCommaParts(author);
	if (parts.length !== 2) return author;
	if (canonicalAuthorCredential(parts[1])) return author;
	return `${parts[1]} ${parts[0]}`.trim();
}

function trailingAuthorCredential(fields: string[]): { index: number; suffix: string } | null {
	const maxWidth = Math.min(3, fields.length);
	for (let width = maxWidth; width >= 1; width--) {
		const index = fields.length - width;
		const suffix = canonicalAuthorCredential(fields.slice(index).join(''));
		if (suffix) return { index, suffix };
	}
	return null;
}

function canonicalAuthorCredential(value: string): string | null {
	const token = normalizedAuthorToken(value);
	const suffix = authorCredentialSuffixes[token];
	if (!suffix || !authorCredentialLooksExplicit(value, token)) return null;
	return suffix;
}

function authorCredentialLooksExplicit(value: string, token: string): boolean {
	if (authorCredentialSuffixesAllowLowercase.has(token)) return true;
	const letters = value.match(/[A-Za-z]/g) ?? [];
	const hasDot = value.includes('.');
	const allLettersUpper = letters.length > 0 && letters.every((letter) => letter === letter.toUpperCase());
	return hasDot || allLettersUpper;
}

function normalizedAuthorToken(value: string): string {
	return value.trim().toLowerCase().replace(/[^a-z0-9]/g, '');
}

function formatAuthorNamePart(author: string): string {
	const fields = author.replace(/\./g, ' ').split(/\s+/).map((part) => part.replace(/,$/, '')).filter(Boolean);
	const parts: string[] = [];
	for (let index = 0; index < fields.length; index++) {
		const field = fields[index];
		if (index === 0) {
			const prefix = authorTitlePrefixes[normalizedAuthorToken(field)];
			if (prefix) {
				parts.push(prefix);
				continue;
			}
		}
		if (/^[A-Za-z]$/.test(field)) {
			parts.push(`${field.toUpperCase()}.`);
			continue;
		}
		if (index === 0 && /^[A-Z]{2,4}$/.test(field)) {
			parts.push(...field.split('').map((letter) => `${letter}.`));
			continue;
		}
		parts.push(field);
	}
	return parts.join(' ');
}

function splitCommaList(value: unknown): string[] {
	if (typeof value === 'string') {
		return value.split(',').map((item) => item.trim()).filter(Boolean);
	}
	return Array.isArray(value) ? value : [];
}

function uniqueMetadataList(values: string[]): string[] {
	const unique: string[] = [];
	const seen = new Set<string>();
	for (const value of values) {
		const key = value.toLowerCase();
		if (seen.has(key)) continue;
		seen.add(key);
		unique.push(value);
	}
	return unique;
}

export function buildMetadataPayload(editForm: MetadataEditForm, authorsList: string[]) {
	return {
		title: editForm.title,
		authors: authorsList.map((author) => normalizeAuthorName(author.trim())).filter(Boolean),
		series: editForm.series,
		series_number: String(editForm.series_number || '').trim(),
		publisher: editForm.publisher,
		pub_date: editForm.pub_date,
		description: editForm.description,
		rating: starsToRating(editForm.rating),
		status: editForm.status,
		genres: uniqueMetadataList(splitCommaList(editForm.genres)),
		tags: uniqueMetadataList(splitCommaList(editForm.tags)),
		isbn: editForm.isbn,
		asin: editForm.asin,
		language: editForm.language,
		page_count: editForm.page_count,
		comic_spread_fallback: editForm.comic_spread_fallback
	};
}
