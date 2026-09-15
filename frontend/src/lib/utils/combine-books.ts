export const MAX_COMBINE_BOOKS = 25;

export type CombineBookSummary = {
	id: number;
	title?: string;
	authors?: string;
	cover_path?: string;
	format?: string;
	percent?: number;
	status?: string;
	library_id?: number;
	last_read_at?: number;
};

export function combineSelectionError(count: number, filteredMode = false): string | null {
	if (filteredMode) {
		return 'Select specific books to combine. Combining all filtered books is not supported.';
	}
	if (count < 2) {
		return 'Select at least two books to combine.';
	}
	if (count > MAX_COMBINE_BOOKS) {
		return `Select at most ${MAX_COMBINE_BOOKS} books to combine.`;
	}
	return null;
}

export function suggestedCombinePrimaryId(books: CombineBookSummary[]): number | null {
	if (books.length === 0) return null;

	const withActivity = [...books].sort((left, right) => {
		const lastRead = (right.last_read_at || 0) - (left.last_read_at || 0);
		if (lastRead !== 0) return lastRead;
		const percent = (right.percent || 0) - (left.percent || 0);
		if (percent !== 0) return percent;
		const statusRank = combineStatusRank(right.status) - combineStatusRank(left.status);
		if (statusRank !== 0) return statusRank;
		const coverRank = Number(Boolean(right.cover_path)) - Number(Boolean(left.cover_path));
		if (coverRank !== 0) return coverRank;
		return left.id - right.id;
	});
	return withActivity[0].id;
}

function combineStatusRank(status?: string): number {
	switch (status) {
		case 'reading':
			return 2;
		case 'finished':
			return 1;
		default:
			return 0;
	}
}

export function uniqueLibraryIds(books: CombineBookSummary[]): number[] {
	const ids = new Set<number>();
	for (const book of books) {
		if (typeof book.library_id === 'number' && book.library_id > 0) {
			ids.add(book.library_id);
		}
	}
	return Array.from(ids);
}
