<script lang="ts">
	import BookCoverFrame from '$lib/components/BookCoverFrame.svelte';
	import { getCoverThumbUrl } from '$lib/utils/covers';
	import {
		imageFileFromClipboard,
		imageFileFromDataTransfer,
		imageFileFromList
	} from '$lib/utils/cover-upload';

	interface Props {
		bookId: number;
		coverPath?: string | null;
		coverUpdatedOn?: number;
		previewSrc?: string | null;
		format?: string;
		title?: string;
		onClose: () => void;
		onSelected: (file: File) => void;
	}

	let {
		bookId,
		coverPath = null,
		coverUpdatedOn,
		previewSrc = null,
		format,
		title = 'book',
		onClose,
		onSelected
	}: Props = $props();

	let fileInput: HTMLInputElement | null = $state(null);
	let dragging = $state(false);
	let dragDepth = $state(0);
	let error = $state('');
	let currentPreview = $derived(
		previewSrc || (coverPath ? getCoverThumbUrl(bookId, 'medium', coverUpdatedOn) : null)
	);

	$effect(() => {
		function onPaste(event: ClipboardEvent) {
			const file = imageFileFromClipboard(event);
			if (file) {
				event.preventDefault();
				selectFile(file);
				return;
			}
			const items = Array.from(event.clipboardData?.items || []);
			const hasFiles = Boolean(event.clipboardData?.files?.length) || items.some((item) => item.kind === 'file');
			if (hasFiles) error = 'Clipboard does not contain an image.';
		}
		window.addEventListener('paste', onPaste);
		return () => window.removeEventListener('paste', onPaste);
	});

	function openFilePicker() {
		fileInput?.click();
	}

	function handleFileInput(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		const file = imageFileFromList(input.files);
		input.value = '';
		if (!file) {
			error = 'Choose an image file.';
			return;
		}
		selectFile(file);
	}

	function handleDragEnter(event: DragEvent) {
		event.preventDefault();
		dragDepth += 1;
		dragging = true;
	}

	function handleDragOver(event: DragEvent) {
		event.preventDefault();
		if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
	}

	function handleDragLeave(event: DragEvent) {
		event.preventDefault();
		dragDepth = Math.max(0, dragDepth - 1);
		if (dragDepth === 0) dragging = false;
	}

	function handleDrop(event: DragEvent) {
		event.preventDefault();
		dragDepth = 0;
		dragging = false;
		const file = imageFileFromDataTransfer(event.dataTransfer);
		if (!file) {
			error = 'Drop an image file.';
			return;
		}
		selectFile(file);
	}

	function selectFile(file: File) {
		error = '';
		onSelected(file);
		onClose();
	}
</script>

<div class="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
	<div class="flex max-h-[90dvh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] shadow-2xl">
		<div class="flex items-start justify-between gap-4 border-b border-[var(--color-surface-border)] px-5 py-4">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-surface-text)]">Set cover</h2>
				<p class="mt-1 text-sm text-[var(--color-surface-text-muted)]">
					Drop an image, paste with Ctrl+V, or browse for a file.
				</p>
			</div>
			<button type="button" onclick={onClose} class="rounded-lg p-2 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-700)] hover:text-[var(--color-surface-text)]" aria-label="Close">
				<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
				</svg>
			</button>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto p-5">
			{#if currentPreview}
				<div class="mb-4 flex justify-center">
					<BookCoverFrame
						src={currentPreview}
						alt={title}
						{format}
						mode="contain"
						frameClass="h-28 w-[4.7rem]"
					/>
				</div>
			{/if}

			<input bind:this={fileInput} type="file" accept="image/*" class="hidden" onchange={handleFileInput} />

			<div
				role="button"
				tabindex="0"
				class="flex w-full flex-col items-center justify-center rounded-xl border-2 border-dashed px-4 py-10 text-center transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--color-surface-overlay)] {dragging ? 'border-[var(--color-primary-400)] bg-[var(--color-primary-500)]/12' : 'border-[var(--color-surface-border)] bg-[var(--color-surface-base)] hover:border-[var(--color-primary-500)]/55 hover:bg-[var(--color-surface-700)]'} cursor-pointer"
				onclick={openFilePicker}
				onkeydown={(event) => {
					if (event.key === 'Enter' || event.key === ' ') {
						event.preventDefault();
						openFilePicker();
					}
				}}
				ondragenter={handleDragEnter}
				ondragover={handleDragOver}
				ondragleave={handleDragLeave}
				ondrop={handleDrop}
			>
				<svg class="mb-3 h-8 w-8 text-[var(--color-primary-400)]" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"></path>
				</svg>
				<p class="text-sm font-medium text-[var(--color-surface-text)]">
					{dragging ? 'Drop image to use' : 'Drop an image here'}
				</p>
				<p class="mt-1 text-xs text-[var(--color-surface-text-muted)]">or click Browse, or press Ctrl+V to paste</p>
				<span
					class="mt-4 inline-flex rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-700)] px-3 py-1.5 text-sm font-medium text-[var(--color-surface-text)]"
				>
					Browse
				</span>
			</div>

			{#if error}
				<p class="mt-3 text-sm text-red-400">{error}</p>
			{/if}
		</div>

		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-surface-border)] px-5 py-4">
			<button
				type="button"
				onclick={onClose}
				class="rounded-lg px-4 py-2 text-sm font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-700)]"
			>
				Cancel
			</button>
		</div>
	</div>
</div>
