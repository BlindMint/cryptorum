<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { page } from '$app/stores';
	import { appActivity, desktopSidebarCollapsed, mobileMenuOpen } from '$lib/stores';
	import LibraryModal from '$lib/components/LibraryModal.svelte';
	import ShelfModal from '$lib/components/ShelfModal.svelte';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

	let { authDisabled = false } = $props<{ authDisabled?: boolean }>();

	interface Library {
		id: number;
		name: string;
		icon: string;
		book_count: number;
		exclude_from_suggestions?: boolean;
		comic_spread_fallback?: string;
		is_importing?: boolean;
		sort_order?: number;
		paths?: string[];
	}

	interface Shelf {
		id: number;
		name: string;
		icon: string;
		is_magic?: number;
		book_count: number;
		sort_order?: number;
	}

	let libraries = $state<Library[]>([]);
	let shelves = $state<Shelf[]>([]);
	let activeLibraryScanJobs = $state<any[]>([]);
	let isLoading = $state(false);
	let refreshTimer: number | null = null;
	let sidebarWidth = $state(232);
	let isResizing = $state(false);
	let activeResizePointerId: number | null = null;
	let activeLibraryMenu = $state<Library | null>(null);
	let libraryMenuPosition = $state({ top: 0, left: 0 });
	let librarySortMode = $state<'name' | 'count' | 'length'>('name');
	let librarySortDir = $state<'asc' | 'desc'>('asc');
	let draggedLibraryId = $state<number | null>(null);
	let libraryDropTargetId = $state<number | null>(null);
	let libraryDropPosition = $state<'before' | 'after'>('before');
	let showLibrarySortMenu = $state(false);
	let draggedShelfId = $state<number | null>(null);
	let shelfDropTargetId = $state<number | null>(null);
	let shelfDropPosition = $state<'before' | 'after'>('before');
	let showShelfModal = $state(false);

	// Library modal state
	let showLibraryModal = $state(false);
	let editingLibrary = $state<Library | null>(null);
	const SIDEBAR_MIN_WIDTH = 200;
	const SIDEBAR_MAX_WIDTH = 400;
	const SIDEBAR_STORAGE_KEY = 'sidebarWidth';
	const LIBRARY_NAME_CACHE_KEY = 'cryptorumLibraryNames';
	const SHELF_SUMMARY_CACHE_KEY = 'cryptorumShelfSummaries';

	function clampSidebarWidth(width: number): number {
		return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
	}

	function setSidebarWidth(width: number, persist = false) {
		const nextWidth = clampSidebarWidth(width);
		sidebarWidth = nextWidth;

		if (persist) {
			localStorage.setItem(SIDEBAR_STORAGE_KEY, String(nextWidth));
		}
	}

	function handleResizeMove(event: PointerEvent) {
		if (!isResizing || event.pointerId !== activeResizePointerId) return;
		setSidebarWidth(event.clientX);
	}

	function stopResize(event?: PointerEvent) {
		if (!isResizing) return;
		if (event && activeResizePointerId !== null && event.pointerId !== activeResizePointerId) {
			return;
		}

		isResizing = false;
		activeResizePointerId = null;
		window.removeEventListener('pointermove', handleResizeMove);
		window.removeEventListener('pointerup', stopResize);
		window.removeEventListener('pointercancel', stopResize);
		document.documentElement.style.cursor = '';
		document.body.style.userSelect = '';
		setSidebarWidth(sidebarWidth, true);
	}

	function startResize(event: PointerEvent) {
		if ($desktopSidebarCollapsed || event.button !== 0) return;

		event.preventDefault();
		event.stopPropagation();
		stopResize();

		isResizing = true;
		activeResizePointerId = event.pointerId;
		setSidebarWidth(event.clientX);
		document.documentElement.style.cursor = 'col-resize';
		document.body.style.userSelect = 'none';
		window.addEventListener('pointermove', handleResizeMove);
		window.addEventListener('pointerup', stopResize);
		window.addEventListener('pointercancel', stopResize);
	}

	// Helper function to check if a navigation item is active
	function isActive(href: string): boolean {
		const currentPath = $page.url.pathname;
		const currentSearch = $page.url.search;
		const currentParams = $page.url.searchParams;

		if (href === '/') {
			return currentPath === '/' && !currentSearch.includes('library=');
		}

		if (href === '/library') {
			return currentPath === '/library' && !currentSearch.includes('library=');
		}

		if (href.startsWith('/library?library=')) {
			const expectedLibrary = new URLSearchParams(href.split('?')[1] || '').get('library');
			return currentPath === '/library' && currentParams.get('library') === expectedLibrary;
		}

		if (href.startsWith('/shelves/')) {
			return currentPath.startsWith('/shelves/') && currentPath === href;
		}

		return currentPath === href;
	}

	function getJobLibraryId(job: any): number | null {
		const payload = job?.payload;
		if (!payload) return null;
		if (typeof payload.library_id === 'number') return payload.library_id;
		if (typeof payload.library_id === 'string') {
			const parsed = Number.parseInt(payload.library_id, 10);
			return Number.isNaN(parsed) ? null : parsed;
		}
		return null;
	}

	function isLibraryScanActive(library: Library): boolean {
		return !!library.is_importing ||
			activeLibraryScanJobs.some((job: any) => getJobLibraryId(job) === library.id);
	}

	$effect(() => {
		activeLibraryScanJobs = $appActivity.activeJobs.filter((job: any) => job.job_type === 'library_scan');
	});

	onMount(async () => {
		const storedWidth = localStorage.getItem(SIDEBAR_STORAGE_KEY);
		if (storedWidth !== null) {
			const parsedWidth = Number.parseInt(storedWidth, 10);
			if (!Number.isNaN(parsedWidth)) {
				setSidebarWidth(parsedWidth);
			}
		}

		appActivity.init();
		await loadData();
		refreshTimer = window.setInterval(() => {
			if (libraries.some((library) => isLibraryScanActive(library)) || activeLibraryScanJobs.length > 0) {
				void loadData();
			}
		}, 3000);

		(window as any).refreshSidebar = loadData;
		document.addEventListener('click', closeLibraryMenu);
	});

	onDestroy(() => {
		stopResize();
		document.removeEventListener('click', closeLibraryMenu);
		if (refreshTimer !== null) {
			window.clearInterval(refreshTimer);
		}
	});

	function openLibraryModal() {
		editingLibrary = null;
		showLibraryModal = true;
	}

	function openShelfModal() {
		showShelfModal = true;
	}

	async function handleShelfSaved() {
		showShelfModal = false;
		await loadData();
	}

	function closeLibraryModal() {
		showLibraryModal = false;
		editingLibrary = null;
	}

	function closeLibraryMenu() {
		activeLibraryMenu = null;
		showLibrarySortMenu = false;
	}

	function closeMobileNavigation() {
		$mobileMenuOpen = false;
		closeLibraryMenu();
	}

	function openLibraryMenu(event: MouseEvent, library: Library) {
		event.preventDefault();
		event.stopPropagation();

		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		const menuWidth = 196;
		const menuHeight = 178;
		const margin = 8;

		libraryMenuPosition = {
			top: Math.min(Math.max(margin, rect.bottom + 4), window.innerHeight - menuHeight - margin),
			left: Math.min(Math.max(margin, rect.right - menuWidth), window.innerWidth - menuWidth - margin)
		};
		activeLibraryMenu = activeLibraryMenu?.id === library.id ? null : library;
	}

	async function openEditLibrary(library: Library | null) {
		if (!library) return;
		closeLibraryMenu();
		try {
			const response = await fetch(`/api/libraries/${library.id}`, { cache: 'no-store' });
			const fullLibrary = response.ok ? await response.json() : library;
			editingLibrary = fullLibrary;
			showLibraryModal = true;
		} catch (e) {
			console.error('Failed to load library for editing:', e);
		}
	}

	async function handleLibrarySaved(result: { library: Library; isEditing: boolean; foldersChanged: boolean }) {
		closeLibraryModal();
		await loadData();
		if (result.isEditing && result.foldersChanged && !result.library.is_importing && confirm('Library folders changed. Scan this library now?')) {
			await scanLibrary(result.library);
		}
	}

	async function scanLibrary(library: any) {
		closeLibraryMenu();
		const pendingJob = appActivity.startPendingJob({
			job_type: 'library_scan',
			title: `Scan library: ${library.name}`,
			payload: { library_id: library.id, library_name: library.name }
		});
		try {
			const response = await fetch(`/api/libraries/${library.id}/scan`, { method: 'POST' });
			if (response.ok) {
				const result = await response.json().catch(() => null);
				if (result?.job_id) {
					appActivity.confirmPendingJob(pendingJob, result.job_id);
				} else {
					appActivity.confirmPendingJob(pendingJob);
				}
				await appActivity.refresh();
				await loadData();
			} else {
				appActivity.failPendingJob(pendingJob, 'Unable to queue library scan.');
			}
		} catch (e) {
			console.error('Failed to scan library:', e);
			appActivity.failPendingJob(pendingJob, 'Unable to queue library scan.');
		}
	}

	async function regenerateLibraryCovers(library: Library | null, mode: 'all' | 'missing' = 'all') {
		if (!library) return;
		if (!confirmBulkAction({
			action: mode === 'all' ? 'regenerate covers for' : 'regenerate missing covers for',
			count: library.book_count
		})) return;
		closeLibraryMenu();
		const pendingJob = appActivity.startPendingJob({
			job_type: 'cover_regenerate',
			title: mode === 'all' ? `Regenerate covers for ${library.name}` : `Regenerate missing covers for ${library.name}`,
			payload: { mode, library_id: library.id, library_name: library.name }
		});
		try {
			const response = await fetch('/api/settings/book-covers/regenerate', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ mode, library_id: library.id })
			});
			if (!response.ok) {
				appActivity.failPendingJob(pendingJob, 'Unable to queue cover regeneration.');
				console.error('Failed to queue library cover regeneration:', await response.text());
				return;
			}
			const job = await response.json().catch(() => null);
			if (job?.id) {
				appActivity.confirmPendingJob(pendingJob, job);
			} else {
				appActivity.confirmPendingJob(pendingJob);
			}
			await appActivity.refresh();
		} catch (e) {
			console.error('Failed to queue library cover regeneration:', e);
			appActivity.failPendingJob(pendingJob, 'Unable to queue cover regeneration.');
		}
	}

	async function loadData() {
		// Prevent concurrent requests
		if (isLoading) return;
		isLoading = true;

		try {
			const [libsRes, shelvesRes] = await Promise.all([
				fetch('/api/libraries', { cache: 'no-store' }),
				fetch('/api/shelves', { cache: 'no-store' })
			]);

			if (libsRes.ok) {
				const libs = await libsRes.json();
				if (libs) {
					libraries = libs;
					cacheLibraryNames(libs);
				}
			}

			if (shelvesRes.ok) {
				const sh = await shelvesRes.json();
				if (sh) {
					shelves = sh;
					cacheShelfSummaries(sh);
				}
			}
			await appActivity.refresh();
		} catch (e) {
			console.error('Failed to load navigation data:', e);
		} finally {
			isLoading = false;
		}
	}

	function cacheLibraryNames(nextLibraries: Library[]) {
		try {
			const names = Object.fromEntries(nextLibraries.map((library) => [String(library.id), library.name || '']));
			sessionStorage.setItem(LIBRARY_NAME_CACHE_KEY, JSON.stringify(names));
		} catch {
			// Ignore storage failures; the library page will still fetch the name.
		}
	}

	function cacheShelfSummaries(nextShelves: Shelf[]) {
		try {
			sessionStorage.setItem(SHELF_SUMMARY_CACHE_KEY, JSON.stringify(nextShelves));
		} catch {
			// Ignore storage failures; shelf pages will still fetch fresh data.
		}
	}

	async function persistLibraryOrder(nextLibraries: Library[]) {
		libraries = nextLibraries;
		try {
			await fetch('/api/libraries/order', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ library_ids: nextLibraries.map((library) => library.id) })
			});
		} catch (e) {
			console.error('Failed to save library order:', e);
			await loadData();
		}
	}

	function applyLibrarySort(mode = librarySortMode, direction = librarySortDir) {
		librarySortMode = mode;
		librarySortDir = direction;
		const multiplier = direction === 'asc' ? 1 : -1;
		const sorted = [...libraries].sort((a, b) => {
			let comparison = 0;
			if (mode === 'count') comparison = (a.book_count || 0) - (b.book_count || 0);
			else if (mode === 'length') comparison = a.name.length - b.name.length;
			else comparison = a.name.localeCompare(b.name);
			if (comparison === 0) comparison = a.name.localeCompare(b.name);
			return comparison * multiplier;
		});
		void persistLibraryOrder(sorted);
	}

	function toggleLibrarySortDirection() {
		applyLibrarySort(librarySortMode, librarySortDir === 'asc' ? 'desc' : 'asc');
	}

	function librarySortLabel() {
		switch (librarySortMode) {
			case 'count':
				return 'Count';
			case 'length':
				return 'Length';
			default:
				return 'Sort';
		}
	}

	function handleLibraryDragStart(event: DragEvent, library: Library) {
		draggedLibraryId = library.id;
		libraryDropTargetId = null;
		event.dataTransfer?.setData('text/plain', String(library.id));
		if (event.currentTarget instanceof Element) {
			event.dataTransfer?.setDragImage(event.currentTarget, 12, 12);
		}
	}

	function handleLibraryDragOver(event: DragEvent, targetLibrary: Library) {
		if (!draggedLibraryId || draggedLibraryId === targetLibrary.id) return;
		event.preventDefault();
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		libraryDropTargetId = targetLibrary.id;
		libraryDropPosition = event.clientY < rect.top + rect.height / 2 ? 'before' : 'after';
	}

	function clearLibraryDragState() {
		draggedLibraryId = null;
		libraryDropTargetId = null;
	}

	function handleLibraryDrop(event: DragEvent, targetLibrary: Library) {
		event.preventDefault();
		const sourceId = draggedLibraryId ?? Number(event.dataTransfer?.getData('text/plain'));
		const dropPosition = libraryDropPosition;
		clearLibraryDragState();
		if (!sourceId || sourceId === targetLibrary.id) return;
		const current = [...libraries];
		const sourceIndex = current.findIndex((library) => library.id === sourceId);
		const targetIndex = current.findIndex((library) => library.id === targetLibrary.id);
		if (sourceIndex === -1 || targetIndex === -1) return;
		const [moved] = current.splice(sourceIndex, 1);
		const adjustedTargetIndex = current.findIndex((library) => library.id === targetLibrary.id);
		const insertIndex = dropPosition === 'after' ? adjustedTargetIndex + 1 : adjustedTargetIndex;
		current.splice(insertIndex, 0, moved);
		void persistLibraryOrder(current);
	}

	async function persistShelfOrder(nextShelves: Shelf[]) {
		shelves = nextShelves;
		try {
			await fetch('/api/shelves/order', {
				method: 'PATCH',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ shelf_ids: nextShelves.map((shelf) => shelf.id) })
			});
		} catch (e) {
			console.error('Failed to save shelf order:', e);
			await loadData();
		}
	}

	function handleShelfDragStart(event: DragEvent, shelf: Shelf) {
		draggedShelfId = shelf.id;
		shelfDropTargetId = null;
		event.dataTransfer?.setData('text/plain', String(shelf.id));
		if (event.currentTarget instanceof Element) {
			event.dataTransfer?.setDragImage(event.currentTarget, 12, 12);
		}
	}

	function handleShelfDragOver(event: DragEvent, targetShelf: Shelf) {
		if (!draggedShelfId || draggedShelfId === targetShelf.id) return;
		event.preventDefault();
		const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
		shelfDropTargetId = targetShelf.id;
		shelfDropPosition = event.clientY < rect.top + rect.height / 2 ? 'before' : 'after';
	}

	function clearShelfDragState() {
		draggedShelfId = null;
		shelfDropTargetId = null;
	}

	function handleShelfDrop(event: DragEvent, targetShelf: Shelf) {
		event.preventDefault();
		const sourceId = draggedShelfId ?? Number(event.dataTransfer?.getData('text/plain'));
		const dropPosition = shelfDropPosition;
		clearShelfDragState();
		if (!sourceId || sourceId === targetShelf.id) return;
		const current = [...shelves];
		const sourceIndex = current.findIndex((shelf) => shelf.id === sourceId);
		const targetIndex = current.findIndex((shelf) => shelf.id === targetShelf.id);
		if (sourceIndex === -1 || targetIndex === -1) return;
		const [moved] = current.splice(sourceIndex, 1);
		const adjustedTargetIndex = current.findIndex((shelf) => shelf.id === targetShelf.id);
		const insertIndex = dropPosition === 'after' ? adjustedTargetIndex + 1 : adjustedTargetIndex;
		current.splice(insertIndex, 0, moved);
		void persistShelfOrder(current);
	}

</script>



<aside
	id="app-sidebar"
	class="
		fixed lg:static top-[var(--app-topbar-height)] lg:top-0 bottom-0 left-0 z-40
		w-[min(20rem,calc(100vw-0.75rem))] bg-[var(--color-surface-overlay)] shadow-[1px_0_0_rgba(255,255,255,0.03)]
		transform overflow-hidden
		lg:translate-x-0
		{$desktopSidebarCollapsed ? 'lg:w-0 lg:min-w-0 lg:shadow-none' : 'lg:w-[var(--sidebar-width)] lg:min-w-[200px]'}
		{$mobileMenuOpen ? 'translate-x-0' : '-translate-x-full'}
		flex flex-col h-[calc(100dvh-var(--app-topbar-height))] lg:h-full min-h-0
	"
	style={`--sidebar-width: ${sidebarWidth}px;`}
>
	<div class={`flex h-full min-w-[200px] flex-col min-h-0 ${$desktopSidebarCollapsed ? 'lg:pointer-events-none lg:opacity-0' : 'lg:opacity-100'}`}>
	<div class="flex-shrink-0 space-y-0.5 p-2 pb-1.5 text-[13px]">
		<a
			href="/"
			onclick={closeMobileNavigation}
			class="flex items-center gap-2 rounded-md px-2 py-1.5 transition-all duration-200 {isActive('/') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
		>
			<svg class="h-4 w-4 shrink-0 transition-transform duration-200 {isActive('/') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"></path>
			</svg>
			<span>Dashboard</span>
		</a>

		<a
			href="/library"
			onclick={closeMobileNavigation}
			class="flex items-center gap-2 rounded-md px-2 py-1.5 transition-all duration-200 {isActive('/library') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
		>
			<svg class="h-4 w-4 shrink-0 transition-transform duration-200 {isActive('/library') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<span>All Books</span>
		</a>

		<a
			href="/authors"
			onclick={closeMobileNavigation}
			class="flex items-center gap-2 rounded-md px-2 py-1.5 transition-all duration-200 {isActive('/authors') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
		>
			<svg class="h-4 w-4 shrink-0 transition-transform duration-200 {isActive('/authors') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
			</svg>
			<span>Authors</span>
		</a>

		<a
			href="/series"
			onclick={closeMobileNavigation}
			class="flex items-center gap-2 rounded-md px-2 py-1.5 transition-all duration-200 {isActive('/series') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
		>
			<svg class="h-4 w-4 shrink-0 transition-transform duration-200 {isActive('/series') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<span>Series</span>
		</a>
	</div>

	<div class="mx-2 h-px bg-[var(--color-surface-border)]"></div>

	<nav class="custom-scrollbar min-h-0 flex-1 space-y-2 overflow-x-hidden overflow-y-auto p-2 text-[13px]">
		<div>
			<div class="flex items-center justify-between px-2 py-1">
				<a
					href="/libraries"
					onclick={closeMobileNavigation}
					class="flex items-center gap-1.5 rounded-md text-[10px] font-semibold uppercase tracking-wider text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
					aria-label="View libraries"
				>
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M3 7l9-4 9 4M4 10h16v11H4V10z"></path>
					</svg>
					<span>Libraries</span>
				</a>
				<div class="flex items-center gap-0.5">
					<div class="relative">
						<div class="inline-flex h-6 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-700)]">
							<button
								type="button"
								onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = !showLibrarySortMenu; }}
								class="min-w-0 px-1.5 text-[11px] font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-600)] focus:outline-none focus:ring-1 focus:ring-inset focus:ring-[var(--color-primary-500)]"
								aria-haspopup="menu"
								aria-expanded={showLibrarySortMenu}
								aria-label="Sort libraries by {librarySortLabel()}"
								title="Sort libraries by {librarySortLabel()}"
							>
								{librarySortLabel()}
							</button>
							<button
								type="button"
								onclick={(event) => { event.stopPropagation(); toggleLibrarySortDirection(); }}
								class="flex w-6 items-center justify-center border-l border-[var(--color-surface-border)] text-[var(--color-primary-400)] transition-colors hover:bg-[var(--color-surface-600)] focus:outline-none focus:ring-1 focus:ring-inset focus:ring-[var(--color-primary-500)]"
								title={librarySortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
								aria-label={librarySortDir === 'asc' ? 'Sort libraries ascending' : 'Sort libraries descending'}
							>
								<span class="text-xs leading-none">{librarySortDir === 'asc' ? '↑' : '↓'}</span>
							</button>
						</div>
						{#if showLibrarySortMenu}
							<div class="absolute right-0 top-full z-[85] mt-1 w-28 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-0.5 shadow-xl" role="menu">
								<button
									type="button"
									class="block w-full px-2.5 py-1.5 text-left text-[11px] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'name' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('name'); }}
								>
									Name
								</button>
								<button
									type="button"
									class="block w-full px-2.5 py-1.5 text-left text-[11px] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'count' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('count'); }}
								>
									Count
								</button>
								<button
									type="button"
									class="block w-full px-2.5 py-1.5 text-left text-[11px] text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'length' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('length'); }}
								>
									Length
								</button>
							</div>
						{/if}
					</div>
					<button
						onclick={openLibraryModal}
						class="rounded p-0.5 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-primary-500)]"
						title="Add Library"
					>
						<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
						</svg>
					</button>
				</div>
			</div>

			{#each libraries as library}
				{@const parsedLibraryIcon = parseLibraryIcon(library.icon || 'book')}
				<div
					role="listitem"
					draggable="true"
					ondragstart={(event) => handleLibraryDragStart(event, library)}
					ondragend={clearLibraryDragState}
					ondragover={(event) => handleLibraryDragOver(event, library)}
					ondrop={(event) => handleLibraryDrop(event, library)}
					class="group/library-row relative flex items-center rounded-md transition-all duration-200 {draggedLibraryId === library.id ? 'opacity-50' : ''} {isActive('/library?library=' + library.id) ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
				>
					{#if libraryDropTargetId === library.id && draggedLibraryId !== library.id}
						<div class="pointer-events-none absolute left-2 right-2 z-10 h-0.5 rounded-full bg-[var(--color-primary-400)] shadow-[0_0_10px_var(--color-primary-500)] {libraryDropPosition === 'before' ? '-top-0.5' : '-bottom-0.5'}"></div>
					{/if}
					<a
						href="/library?library={library.id}"
						onclick={closeMobileNavigation}
						class="flex min-w-0 flex-1 items-center gap-2 px-2 py-1"
					>
							{#if isLibraryScanActive(library)}
								<svg class="animate-scan-spin h-4 w-4 flex-shrink-0 text-[var(--color-primary-500)] transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
								</svg>
							{:else}
								<div class="sidebar-icon-svg h-4 w-4 flex-shrink-0 text-[var(--color-primary-500)] transition-transform duration-200 group-hover/library-row:hidden">
									{#if parsedLibraryIcon?.svg}
										{@html parsedLibraryIcon.svg}
									{:else}
										<svg class="h-full w-full" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
										</svg>
									{/if}
								</div>
								<svg class="hidden h-4 w-4 flex-shrink-0 cursor-grab text-[var(--color-surface-text-muted)] group-hover/library-row:block" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<circle cx="9" cy="7" r="1.5"></circle>
									<circle cx="15" cy="7" r="1.5"></circle>
									<circle cx="9" cy="12" r="1.5"></circle>
									<circle cx="15" cy="12" r="1.5"></circle>
									<circle cx="9" cy="17" r="1.5"></circle>
									<circle cx="15" cy="17" r="1.5"></circle>
								</svg>
							{/if}
							<span class="min-w-0 flex-1 truncate">{library.name}</span>
					</a>
					<button
						type="button"
						class="group/count relative mr-1 inline-flex h-5 min-w-6 flex-shrink-0 items-center justify-center rounded px-1.5 text-[11px] font-medium text-[var(--color-surface-500)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)] bg-[var(--color-surface-700)]"
						aria-haspopup="menu"
						aria-expanded={activeLibraryMenu?.id === library.id}
						aria-label="Open actions for {library.name}"
						title="Library actions"
						onclick={(event) => openLibraryMenu(event, library)}
					>
						<span class="transition-opacity group-hover/count:opacity-0 group-focus/count:opacity-0 {activeLibraryMenu?.id === library.id ? 'opacity-0' : ''}">{library.book_count}</span>
						<svg class="absolute h-3.5 w-3.5 opacity-0 transition-opacity group-hover/count:opacity-100 group-focus/count:opacity-100 {activeLibraryMenu?.id === library.id ? 'opacity-100' : ''}" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
							<circle cx="12" cy="5" r="2"></circle>
							<circle cx="12" cy="12" r="2"></circle>
							<circle cx="12" cy="19" r="2"></circle>
						</svg>
					</button>
				</div>
			{/each}
		</div>

		<div>
			<div class="flex items-center justify-between px-2 py-1">
				<a
					href="/shelves"
					onclick={closeMobileNavigation}
					class="flex items-center gap-1.5 rounded-md text-[10px] font-semibold uppercase tracking-wider text-[var(--color-surface-text-muted)] transition-colors hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
					aria-label="View shelves"
				>
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"></path>
					</svg>
					<span>Shelves</span>
				</a>
				<button
					type="button"
					onclick={() => { openShelfModal(); closeMobileNavigation(); }}
					class="rounded p-0.5 text-[var(--color-surface-text-muted)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-primary-500)]"
					title="Create Shelf"
				>
					<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
					</svg>
				</button>
			</div>

			{#each shelves as shelf}
				{@const parsedShelfIcon = parseLibraryIcon(shelf.icon || (shelf.is_magic === 1 ? 'sparkles' : 'bookmark'))}
				<div
					role="listitem"
					draggable="true"
					ondragstart={(event) => handleShelfDragStart(event, shelf)}
					ondragend={clearShelfDragState}
					ondragover={(event) => handleShelfDragOver(event, shelf)}
					ondrop={(event) => handleShelfDrop(event, shelf)}
					class="group/shelf-row relative flex items-center rounded-md transition-all duration-200 {draggedShelfId === shelf.id ? 'opacity-50' : ''} {isActive('/shelves/' + shelf.id) ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-0.5 hover:shadow-sm'}"
				>
					{#if shelfDropTargetId === shelf.id && draggedShelfId !== shelf.id}
						<div class="pointer-events-none absolute left-2 right-2 z-10 h-0.5 rounded-full bg-[var(--color-primary-400)] shadow-[0_0_10px_var(--color-primary-500)] {shelfDropPosition === 'before' ? '-top-0.5' : '-bottom-0.5'}"></div>
					{/if}
					<a
						href="/shelves/{shelf.id}"
						class="flex min-w-0 flex-1 items-center gap-2 px-2 py-1"
						onclick={closeMobileNavigation}
					>
						<div class="sidebar-icon-svg h-4 w-4 flex-shrink-0 text-[var(--color-primary-400)] transition-transform duration-200 group-hover/shelf-row:hidden">
							{#if parsedShelfIcon?.svg}
								{@html parsedShelfIcon.svg}
							{:else}
								<svg class="h-full w-full" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
								</svg>
							{/if}
						</div>
						<svg class="hidden h-4 w-4 flex-shrink-0 cursor-grab text-[var(--color-surface-text-muted)] group-hover/shelf-row:block" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
							<circle cx="9" cy="7" r="1.5"></circle>
							<circle cx="15" cy="7" r="1.5"></circle>
							<circle cx="9" cy="12" r="1.5"></circle>
							<circle cx="15" cy="12" r="1.5"></circle>
							<circle cx="9" cy="17" r="1.5"></circle>
							<circle cx="15" cy="17" r="1.5"></circle>
						</svg>
						<span class="min-w-0 flex-1 truncate">{shelf.name}</span>
						{#if shelf.is_magic === 1}
							<span
								class="inline-flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center text-purple-300"
								title="Magic shelf"
								aria-label="Magic shelf"
							>
								<svg class="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z"></path>
								</svg>
							</span>
						{/if}
					</a>
					<span class="mr-1 inline-flex h-5 min-w-6 flex-shrink-0 items-center justify-center rounded bg-[var(--color-surface-700)] px-1.5 text-[11px] font-medium text-[var(--color-surface-500)]">
						{shelf.book_count}
					</span>
				</div>
			{/each}
		</div>
	</nav>

	{#if !authDisabled}
		<footer class="flex-shrink-0 space-y-0.5 border-t border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] p-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] text-[13px]">
			<button
				onclick={async () => {
					await fetch('/api/auth/logout', { method: 'POST' });
					window.location.href = '/login';
				}}
				class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-[var(--color-surface-text)] transition-colors hover:bg-red-500/20 hover:text-red-400"
			>
				<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 013-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"></path>
				</svg>
				<span>Logout</span>
			</button>
		</footer>
	{/if}
</div>
	<div
		class={`group absolute inset-y-0 -right-1 hidden w-3 lg:flex cursor-col-resize touch-none select-none ${$desktopSidebarCollapsed ? 'lg:pointer-events-none lg:opacity-0' : ''}`}
		title="Drag to resize sidebar"
		role="separator"
		aria-orientation="vertical"
		aria-label="Resize sidebar"
		onpointerdown={startResize}
	>
		<div class={`ml-auto h-full w-px bg-transparent transition-colors group-hover:bg-[var(--color-primary-500)]/55 ${isResizing ? 'bg-[var(--color-primary-500)]/70' : ''}`}></div>
	</div>
</aside>

{#if activeLibraryMenu}
	<div
		class="fixed z-[90] w-[196px] overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-1 shadow-2xl"
		style:top={`${libraryMenuPosition.top}px`}
		style:left={`${libraryMenuPosition.left}px`}
		role="menu"
		tabindex="-1"
		aria-label="Actions for {activeLibraryMenu.name}"
		onclick={(event) => event.stopPropagation()}
		onkeydown={(event) => event.stopPropagation()}
	>
		<a
			href="/library?library={activeLibraryMenu.id}"
			class="block px-2.5 py-1.5 text-[13px] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => { closeLibraryMenu(); $mobileMenuOpen = false; }}
		>
			Open Library
		</a>
		<button
			type="button"
			class="block w-full px-2.5 py-1.5 text-left text-[13px] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => openEditLibrary(activeLibraryMenu)}
		>
			Edit Library
		</button>
		<button
			type="button"
			class="block w-full px-2.5 py-1.5 text-left text-[13px] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] disabled:cursor-not-allowed disabled:opacity-60"
			role="menuitem"
			disabled={isLibraryScanActive(activeLibraryMenu)}
			onclick={() => scanLibrary(activeLibraryMenu)}
		>
			{isLibraryScanActive(activeLibraryMenu) ? 'Scan Running' : 'Scan Library'}
		</button>
		<button
			type="button"
			class="block w-full px-2.5 py-1.5 text-left text-[13px] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => regenerateLibraryCovers(activeLibraryMenu, 'all')}
		>
			Regenerate Covers
		</button>
		<button
			type="button"
			class="block w-full px-2.5 py-1.5 text-left text-[13px] text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => regenerateLibraryCovers(activeLibraryMenu, 'missing')}
		>
			Regenerate Missing Covers
		</button>
		<a
			href="/settings?tab=general"
			class="block px-2.5 py-1.5 text-[13px] text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
			role="menuitem"
			onclick={closeLibraryMenu}
		>
			Manage Libraries
		</a>
	</div>
{/if}

<ShelfModal
	open={showShelfModal}
	mode="create"
	initialMagic={false}
	onClose={() => showShelfModal = false}
	onSaved={handleShelfSaved}
/>

<LibraryModal
	open={showLibraryModal}
	library={editingLibrary}
	isScanActive={(library) => isLibraryScanActive(library as Library)}
	onClose={closeLibraryModal}
	onSaved={(result) => handleLibrarySaved(result as { library: Library; isEditing: boolean; foldersChanged: boolean })}
	onScan={(library) => scanLibrary(library)}
	onRegenerateCovers={(library, mode) => regenerateLibraryCovers(library as Library, mode)}
/>

<style>
	.sidebar-icon-svg :global(svg) {
		display: block;
		width: 100%;
		height: 100%;
		overflow: visible;
	}

</style>
