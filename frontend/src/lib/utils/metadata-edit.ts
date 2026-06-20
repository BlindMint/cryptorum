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
	const commaIndex = name.indexOf(',');
	if (commaIndex > 0) {
		const lastName = name.substring(0, commaIndex).trim();
		const firstName = name.substring(commaIndex + 1).trim();
		return `${firstName} ${lastName}`;
	}

	return name;
}

function splitCommaList(value: unknown): string[] {
	if (typeof value === 'string') {
		return value.split(',').map((item) => item.trim()).filter(Boolean);
	}
	return Array.isArray(value) ? value : [];
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
		genres: splitCommaList(editForm.genres),
		tags: splitCommaList(editForm.tags),
		isbn: editForm.isbn,
		asin: editForm.asin,
		language: editForm.language,
		page_count: editForm.page_count,
		comic_spread_fallback: editForm.comic_spread_fallback
	};
}
