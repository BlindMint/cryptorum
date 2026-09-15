const IMAGE_EXTENSIONS = /\.(png|jpe?g|gif|webp|bmp|avif|svg)$/i;

export type CoverMutation = {
	cover_path?: string;
	cover_source?: string;
	cover_updated_on?: number;
};

export type PendingCover = {
	file: File | null;
	previewUrl: string | null;
	reset: boolean;
};

export function createPendingCover(): PendingCover {
	return { file: null, previewUrl: null, reset: false };
}

export function discardPendingCover(pending: PendingCover): PendingCover {
	if (pending.previewUrl) URL.revokeObjectURL(pending.previewUrl);
	return createPendingCover();
}

export function stagePendingCoverFile(pending: PendingCover, file: File): PendingCover {
	if (pending.previewUrl) URL.revokeObjectURL(pending.previewUrl);
	return {
		file,
		previewUrl: URL.createObjectURL(file),
		reset: false
	};
}

export function stagePendingCoverReset(
	pending: PendingCover,
	originalIsCustom: boolean,
	previewUrl: string | null = null
): PendingCover {
	const hadStagedFile = Boolean(pending.file);
	const cleared = discardPendingCover(pending);
	if (hadStagedFile || !originalIsCustom) {
		if (previewUrl) URL.revokeObjectURL(previewUrl);
		return cleared;
	}
	return { file: null, previewUrl, reset: true };
}

export async function fetchSourceCoverPreview(bookId: number): Promise<string | null> {
	try {
		const response = await fetch(`/api/books/${bookId}/cover/source`);
		if (!response.ok) return null;
		const blob = await response.blob();
		if (!blob.size) return null;
		return URL.createObjectURL(blob);
	} catch (reason) {
		console.error('Failed to preview source cover:', reason);
		return null;
	}
}

export function pendingCoverDirty(pending: PendingCover): boolean {
	return Boolean(pending.file || pending.reset);
}

export function pendingCoverDisplaySrc(pending: PendingCover, fallbackSrc: string | null): string | null {
	if (pending.previewUrl) return pending.previewUrl;
	if (pending.reset) return null;
	return fallbackSrc;
}

export function pendingCoverIsCustom(pending: PendingCover, originalSource?: string | null): boolean {
	if (pending.file) return true;
	if (pending.reset) return false;
	return originalSource === 'custom';
}

export async function persistPendingCover(
	bookId: number,
	pending: PendingCover
): Promise<{ update: CoverMutation | null; error: string | null }> {
	if (pending.file) {
		const formData = new FormData();
		formData.append('cover', pending.file);
		try {
			const response = await fetch(`/api/books/${bookId}/cover/custom`, {
				method: 'POST',
				body: formData
			});
			if (!response.ok) {
				return { update: null, error: (await response.text()) || 'Failed to upload cover.' };
			}
			return { update: (await response.json()) as CoverMutation, error: null };
		} catch (reason) {
			console.error('Failed to upload cover:', reason);
			return { update: null, error: 'Failed to upload cover.' };
		}
	}
	if (pending.reset) {
		try {
			const response = await fetch(`/api/books/${bookId}/cover/custom`, { method: 'DELETE' });
			if (!response.ok) {
				return { update: null, error: (await response.text()) || 'Failed to reset cover.' };
			}
			return { update: (await response.json()) as CoverMutation, error: null };
		} catch (reason) {
			console.error('Failed to reset cover:', reason);
			return { update: null, error: 'Failed to reset cover.' };
		}
	}
	return { update: null, error: null };
}

export function isImageFile(file: File | null | undefined): file is File {
	if (!file) return false;
	if (file.type.startsWith('image/')) return true;
	return IMAGE_EXTENSIONS.test(file.name);
}

export function imageFileFromList(files: FileList | File[] | null | undefined): File | null {
	if (!files) return null;
	for (const file of Array.from(files)) {
		if (isImageFile(file)) return file;
	}
	return null;
}

export function imageFileFromClipboard(event: ClipboardEvent): File | null {
	const data = event.clipboardData;
	if (!data) return null;
	const fromFiles = imageFileFromList(data.files);
	if (fromFiles) return fromFiles;
	for (const item of Array.from(data.items || [])) {
		if (item.kind !== 'file') continue;
		const file = item.getAsFile();
		if (isImageFile(file)) return file;
	}
	return null;
}

export function imageFileFromDataTransfer(data: DataTransfer | null | undefined): File | null {
	if (!data) return null;
	return imageFileFromList(data.files);
}
