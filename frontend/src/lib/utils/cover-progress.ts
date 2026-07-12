import { isAudioFormat } from '$lib/utils/book-formats';

/** True when a book should show cover progress (bar and optional chip). */
export function bookHasCoverProgress(book: { opened?: boolean | number | null; percent?: number | null } | null | undefined): boolean {
	if (!book) return false;
	const opened = book.opened === true || book.opened === 1;
	return opened && (Number(book.percent) || 0) > 0;
}

export function formatCoverProgressPercent(percent: number | null | undefined): number {
	return Math.max(0, Math.min(100, Math.round(Number(percent) || 0)));
}

export function getProgressChipAriaLabel(
	title: string | null | undefined,
	percent: number | null | undefined,
	format?: string | null
): string {
	const name = title || 'book';
	const rounded = formatCoverProgressPercent(percent);
	if (isAudioFormat(format)) {
		return `Continue listening to ${name}, ${rounded}%`;
	}
	return `Continue reading ${name}, ${rounded}%`;
}
