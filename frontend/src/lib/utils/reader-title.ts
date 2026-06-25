export type ReaderFile = {
	path?: string;
	format?: string;
};

function cleanTitle(value: unknown): string {
	return String(value || '').trim();
}

function fileStem(path: string): string {
	const fileName = path.split(/[\\/]/).pop() || path;
	return fileName.replace(/\.[^.]+$/, '').trim();
}

function preferredFile(files: ReaderFile[], format?: string | null): ReaderFile | null {
	if (files.length === 0) return null;
	const normalizedFormat = cleanTitle(format).toLowerCase();
	if (normalizedFormat) {
		const match = files.find((file) => cleanTitle(file.format).toLowerCase() === normalizedFormat);
		if (match) return match;
	}
	return files[0];
}

export function getReaderDisplayTitle(
	book: any,
	files: ReaderFile[] = [],
	loading = false,
	format?: string | null
): string {
	const title = cleanTitle(book?.title || book?.metadata?.title);
	if (title) return title;

	const directPath = cleanTitle(book?.path || book?.file_path || book?.filename || book?.file_name);
	if (directPath) return fileStem(directPath);

	const file = preferredFile(files, format);
	const path = cleanTitle(file?.path);
	if (path) return fileStem(path);

	return book || !loading ? 'Untitled' : 'Loading...';
}
