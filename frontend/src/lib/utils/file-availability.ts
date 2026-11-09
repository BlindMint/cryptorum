export type FileAvailability = { code: string; message?: string };
export type AvailableFile = { availability?: FileAvailability };

export function isFileAvailable(file: AvailableFile): boolean {
	return !file.availability || file.availability.code === 'available';
}

export function requireReadableFile(file: AvailableFile | null): void {
	if (!file) throw new Error('This book’s file is unavailable. Check its library folder and try again. Your saved reading progress is preserved.');
	if (!isFileAvailable(file)) throw new Error(file.availability?.message || 'This book’s file is unavailable.');
}

export async function fileResponseError(response: Response): Promise<string> {
	try {
		const body = await response.json();
		if (body.error) return body.error;
	} catch { /* Proxies may return non-JSON errors. */ }
	if (response.status === 401) return 'Your session has expired. Sign in again to read this book.';
	if (response.status === 403) return 'Access to this book was denied. Check your permissions.';
	if (response.status === 404) return 'This book’s file is unavailable. Check its library folder and try again.';
	return 'The book could not be loaded. Check your connection and library storage, then try again.';
}

// A ranged GET also works on API routes that do not expose HEAD.
export async function checkReaderDelivery(url: string): Promise<void> {
	const response = await fetch(url, { headers: { Range: 'bytes=0-0' }, cache: 'no-store' });
	if (!response.ok) throw new Error(await fileResponseError(response));
	await response.body?.cancel();
}
