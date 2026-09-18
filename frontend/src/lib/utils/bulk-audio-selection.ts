export type AudioEligibleBook = {
	id: number;
	has_audio?: boolean;
};

export function selectionMayContainAudio(
	selectedIds: Iterable<number>,
	books: AudioEligibleBook[]
): boolean {
	const selected = Array.from(selectedIds, Number);
	if (selected.length === 0) return false;

	const booksById = new Map(books.map((book) => [Number(book.id), book]));
	for (const id of selected) {
		const book = booksById.get(id);
		if (!book || typeof book.has_audio !== 'boolean') return true;
		if (book.has_audio) return true;
	}

	return false;
}
