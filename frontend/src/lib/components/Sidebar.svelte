<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { page } from '$app/stores';
	import { appActivity, desktopSidebarCollapsed, mobileMenuOpen } from '$lib/stores';
	import LibraryIconPicker from '$lib/components/LibraryIconPicker.svelte';
	import { confirmBulkAction } from '$lib/utils/bulk-confirm';
	import { parseLibraryIcon } from '$lib/utils/library-icons';

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
	}

	let libraries = $state<Library[]>([]);
	let shelves = $state<Shelf[]>([]);
	let activeLibraryScanJobs = $state<any[]>([]);
	let isLoading = $state(false);
	let isCreating = $state(false);
	let refreshTimer: number | null = null;
	let sidebarWidth = $state(256);
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

	// Library modal state
	let showLibraryModal = $state(false);
	let editingLibrary = $state<Library | null>(null);
	let showLibraryIconPicker = $state(false);
	let libraryForm = $state({
		name: '',
		icon: '',
		exclude_from_suggestions: false,
		comic_spread_fallback: 'inherit',
		paths: ['']
	});
	let originalLibraryPaths = $state<string[]>([]);
	let currentLibraryIcon = $derived(parseLibraryIcon(libraryForm.icon));

	// Directory browser state
	let showDirectoryModal = $state(false);
	let currentDirectory = $state('/');
	let directoryContents = $state<any[]>([]);
	let directoryLoading = $state(false);
	const SIDEBAR_MIN_WIDTH = 240;
	const SIDEBAR_MAX_WIDTH = 400;
	const SIDEBAR_STORAGE_KEY = 'sidebarWidth';
	const inheritedComicSpreadFallbackOptions = [
		{ value: 'inherit', label: 'Inherit' },
		{ value: 'right', label: 'Right side' },
		{ value: 'left', label: 'Left side' },
		{ value: 'disabled', label: 'Disabled' }
	];

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
		originalLibraryPaths = [];
		libraryForm = { name: '', icon: '', exclude_from_suggestions: false, comic_spread_fallback: 'inherit', paths: [''] };
		showLibraryModal = true;
	}

	function closeLibraryModal() {
		showLibraryModal = false;
		editingLibrary = null;
		showLibraryIconPicker = false;
		originalLibraryPaths = [];
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
			originalLibraryPaths = normalizeLibraryPaths(fullLibrary.paths || []);
			libraryForm = {
				name: fullLibrary.name || '',
				icon: fullLibrary.icon || '',
				exclude_from_suggestions: !!fullLibrary.exclude_from_suggestions,
				comic_spread_fallback: fullLibrary.comic_spread_fallback || 'inherit',
				paths: fullLibrary.paths?.length ? [...fullLibrary.paths] : ['']
			};
			showLibraryModal = true;
		} catch (e) {
			console.error('Failed to load library for editing:', e);
		}
	}

	function normalizeLibraryPaths(paths: string[]): string[] {
		return Array.from(new Set(paths.map((path) => path.trim()).filter(Boolean))).sort();
	}

	function libraryFolderSetChanged(nextPaths: string[]): boolean {
		const next = normalizeLibraryPaths(nextPaths);
		if (next.length !== originalLibraryPaths.length) return true;
		return next.some((path, index) => path !== originalLibraryPaths[index]);
	}

	function openLibraryIconPicker() {
		showLibraryIconPicker = true;
	}

	function closeLibraryIconPicker() {
		showLibraryIconPicker = false;
	}

	function selectLibraryIcon(iconValue: string) {
		libraryForm.icon = iconValue;
	}

	function clearLibraryIcon() {
		libraryForm.icon = '';
	}

	function removeLibraryPath(index: number) {
		if (libraryForm.paths.length > 1) {
			libraryForm.paths.splice(index, 1);
			libraryForm.paths = [...libraryForm.paths];
		}
	}

	async function saveLibrary() {
		if (!libraryForm.name.trim()) return;

		isCreating = true;
		try {
			const filteredPaths = libraryForm.paths.filter(p => p.trim());
			const libraryBeingEdited = editingLibrary;
			const foldersChanged = !!libraryBeingEdited && libraryFolderSetChanged(filteredPaths);
			const response = await fetch(libraryBeingEdited ? `/api/libraries/${libraryBeingEdited.id}` : '/api/libraries', {
				method: libraryBeingEdited ? 'PUT' : 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ ...libraryForm, paths: filteredPaths })
			});

			if (response.ok) {
				const savedLibrary = libraryBeingEdited ? libraryBeingEdited : await response.json();
				closeLibraryModal();

				// Reload data to get is_importing state and updated book_count
				await loadData();

				// Trigger a scan for the new library
				if (!libraryBeingEdited) {
					await scanLibrary(savedLibrary);
				} else if (foldersChanged && !libraryBeingEdited.is_importing && confirm('Library folders changed. Scan this library now?')) {
					await scanLibrary(libraryBeingEdited);
				}
			} else {
				console.error(`Failed to ${libraryBeingEdited ? 'update' : 'create'} library`);
			}
		} catch (error) {
			console.error('Error saving library:', error);
		} finally {
			isCreating = false;
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

	async function openDirectoryModal() {
		showDirectoryModal = true;
		currentDirectory = '/books';
		directoryContents = [];
		directoryLoading = true;
		try {
			const response = await fetch('/api/directories?path=/books');
			if (response.ok) {
				await loadDirectoryContents('/books');
			} else {
				await loadDirectoryContents('/');
			}
		} catch (e) {
			await loadDirectoryContents('/');
		}
	}

	function closeDirectoryModal() {
		showDirectoryModal = false;
	}

	async function loadDirectoryContents(path: string) {
		currentDirectory = path;
		directoryLoading = true;
		directoryContents = [];
		try {
			const response = await fetch(`/api/directories?path=${encodeURIComponent(path)}`);
			if (response.ok) {
				directoryContents = await response.json();
			} else {
				directoryContents = [];
			}
		} catch (e) {
			directoryContents = [];
		} finally {
			directoryLoading = false;
		}
	}

	function selectDirectory(item: any) {
		if (item.type === 'directory') {
			loadDirectoryContents(item.path);
		}
	}

	function addSelectedDirectory() {
		const newPath = currentDirectory;
		if (!libraryForm.paths.includes(newPath)) {
			libraryForm.paths = [...libraryForm.paths, newPath];
		}
		closeDirectoryModal();
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
				if (libs) libraries = libs;
			}

			if (shelvesRes.ok) {
				const sh = await shelvesRes.json();
				if (sh) shelves = sh;
			}
			await appActivity.refresh();
		} catch (e) {
			console.error('Failed to load navigation data:', e);
		} finally {
			isLoading = false;
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

</script>



<aside
	id="app-sidebar"
	class="
		fixed lg:static top-[4.75rem] lg:top-0 bottom-0 left-0 z-40
		w-64 bg-[var(--color-surface-overlay)] shadow-[1px_0_0_rgba(255,255,255,0.03)]
		transform overflow-hidden
		lg:translate-x-0
		{$desktopSidebarCollapsed ? 'lg:w-0 lg:min-w-0 lg:shadow-none' : 'lg:w-[var(--sidebar-width)] lg:min-w-[240px]'}
		{$mobileMenuOpen ? 'translate-x-0' : '-translate-x-full'}
		flex flex-col h-[calc(100dvh-4.75rem)] lg:h-full min-h-0
	"
	style={`--sidebar-width: ${sidebarWidth}px;`}
>
	<div class={`flex h-full min-w-[240px] flex-col min-h-0 ${$desktopSidebarCollapsed ? 'lg:pointer-events-none lg:opacity-0' : 'lg:opacity-100'}`}>
	<div class="flex-shrink-0 p-4 pb-3 space-y-1">
		<a
			href="/"
			onclick={closeMobileNavigation}
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"></path>
			</svg>
			<span>Dashboard</span>
		</a>

		<a
			href="/library"
			onclick={closeMobileNavigation}
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/library') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/library') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<span>All Books</span>
		</a>

		<a
			href="/authors"
			onclick={closeMobileNavigation}
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/authors') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/authors') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
			</svg>
			<span>Authors</span>
		</a>

		<a
			href="/series"
			onclick={closeMobileNavigation}
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/series') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/series') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<span>Series</span>
		</a>
	</div>

	<div class="h-px bg-[var(--color-surface-border)] mx-4"></div>

	<nav class="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-4 space-y-4 custom-scrollbar">
		<div>
			<div class="flex items-center justify-between px-3 py-2">
				<div class="flex items-center space-x-2 text-xs font-semibold text-[var(--color-surface-text-muted)] uppercase tracking-wider">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 14v3m4-3v3m4-3v3M3 21h18M3 10h18M3 7l9-4 9 4M4 10h16v11H4V10z"></path>
					</svg>
					<span>Libraries</span>
				</div>
				<div class="flex items-center gap-1">
					<div class="relative">
						<div class="inline-flex h-7 overflow-hidden rounded-md border border-[var(--color-surface-border)] bg-[var(--color-surface-700)]">
							<button
								type="button"
								onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = !showLibrarySortMenu; }}
								class="min-w-0 px-2 text-xs font-medium text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-600)] focus:outline-none focus:ring-1 focus:ring-inset focus:ring-[var(--color-primary-500)]"
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
								class="flex w-7 items-center justify-center border-l border-[var(--color-surface-border)] text-[var(--color-primary-400)] transition-colors hover:bg-[var(--color-surface-600)] focus:outline-none focus:ring-1 focus:ring-inset focus:ring-[var(--color-primary-500)]"
								title={librarySortDir === 'asc' ? 'Sort ascending' : 'Sort descending'}
								aria-label={librarySortDir === 'asc' ? 'Sort libraries ascending' : 'Sort libraries descending'}
							>
								<span class="text-sm leading-none">{librarySortDir === 'asc' ? '↑' : '↓'}</span>
							</button>
						</div>
						{#if showLibrarySortMenu}
							<div class="absolute right-0 top-full z-[85] mt-1 w-28 overflow-hidden rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] py-1 shadow-xl" role="menu">
								<button
									type="button"
									class="block w-full px-3 py-2 text-left text-xs text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'name' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('name'); }}
								>
									Name
								</button>
								<button
									type="button"
									class="block w-full px-3 py-2 text-left text-xs text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'count' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('count'); }}
								>
									Count
								</button>
								<button
									type="button"
									class="block w-full px-3 py-2 text-left text-xs text-[var(--color-surface-text)] transition-colors hover:bg-[var(--color-surface-base)] {librarySortMode === 'length' ? 'text-[var(--color-primary-300)]' : ''}"
									onclick={(event) => { event.stopPropagation(); showLibrarySortMenu = false; applyLibrarySort('length'); }}
								>
									Length
								</button>
							</div>
						{/if}
					</div>
					<button
						onclick={openLibraryModal}
						class="p-1 rounded text-[var(--color-surface-text-muted)] hover:text-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)] transition-colors"
						title="Add Library"
					>
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
						</svg>
					</button>
				</div>
			</div>

			{#each libraries as library}
				<div
					role="listitem"
					draggable="true"
					ondragstart={(event) => handleLibraryDragStart(event, library)}
					ondragend={clearLibraryDragState}
					ondragover={(event) => handleLibraryDragOver(event, library)}
					ondrop={(event) => handleLibraryDrop(event, library)}
					class="group/library-row relative flex items-center rounded-lg transition-all duration-200 {draggedLibraryId === library.id ? 'opacity-50' : ''} {isActive('/library?library=' + library.id) ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
				>
					{#if libraryDropTargetId === library.id && draggedLibraryId !== library.id}
						<div class="pointer-events-none absolute left-2 right-2 z-10 h-0.5 rounded-full bg-[var(--color-primary-400)] shadow-[0_0_10px_var(--color-primary-500)] {libraryDropPosition === 'before' ? '-top-1' : '-bottom-1'}"></div>
					{/if}
					<a
						href="/library?library={library.id}"
						onclick={closeMobileNavigation}
						class="flex min-w-0 flex-1 items-center space-x-3 px-3 py-2"
					>
							{#if isLibraryScanActive(library)}
								<svg class="animate-scan-spin w-5 h-5 text-[var(--color-primary-500)] flex-shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
								</svg>
							{:else}
								<svg class="w-5 h-5 text-[var(--color-primary-500)] flex-shrink-0 transition-transform duration-200 group-hover/library-row:hidden" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
								</svg>
								<svg class="hidden h-5 w-5 flex-shrink-0 cursor-grab text-[var(--color-surface-text-muted)] group-hover/library-row:block" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<circle cx="9" cy="7" r="1.5"></circle>
									<circle cx="15" cy="7" r="1.5"></circle>
									<circle cx="9" cy="12" r="1.5"></circle>
									<circle cx="15" cy="12" r="1.5"></circle>
									<circle cx="9" cy="17" r="1.5"></circle>
									<circle cx="15" cy="17" r="1.5"></circle>
								</svg>
							{/if}
							<span class="truncate flex-1 min-w-0">{library.name}</span>
					</a>
					<button
						type="button"
						class="group/count relative mr-2 inline-flex h-7 min-w-8 flex-shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-700)] px-2 text-xs font-medium text-[var(--color-surface-500)] transition-colors hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary-500)]"
						aria-haspopup="menu"
						aria-expanded={activeLibraryMenu?.id === library.id}
						aria-label="Open actions for {library.name}"
						title="Library actions"
						onclick={(event) => openLibraryMenu(event, library)}
					>
						<span class="transition-opacity group-hover/count:opacity-0 group-focus/count:opacity-0 {activeLibraryMenu?.id === library.id ? 'opacity-0' : ''}">{library.book_count}</span>
						<svg class="absolute h-4 w-4 opacity-0 transition-opacity group-hover/count:opacity-100 group-focus/count:opacity-100 {activeLibraryMenu?.id === library.id ? 'opacity-100' : ''}" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
							<circle cx="12" cy="5" r="2"></circle>
							<circle cx="12" cy="12" r="2"></circle>
							<circle cx="12" cy="19" r="2"></circle>
						</svg>
					</button>
				</div>
			{/each}
		</div>

		<div>
			<div class="flex items-center justify-between px-3 py-2">
				<div class="flex items-center space-x-2 text-xs font-semibold text-[var(--color-surface-text-muted)] uppercase tracking-wider">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"></path>
					</svg>
					<span>Shelves</span>
				</div>
				<a
					href="/shelves"
					onclick={closeMobileNavigation}
					class="p-1 rounded text-[var(--color-surface-text-muted)] hover:text-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)] transition-colors"
					title="Manage Shelves"
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
					</svg>
				</a>
			</div>

			{#each shelves as shelf}
				<a
					href="/shelves/{shelf.id}"
					class="flex items-center px-3 py-2 rounded-lg transition-all duration-200 {isActive('/shelves/' + shelf.id) ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
					onclick={closeMobileNavigation}
				>
					<div class="flex items-center space-x-3 flex-1 min-w-0">
						<svg class="w-5 h-5 text-[var(--color-primary-400)] flex-shrink-0 transition-transform duration-200 {isActive('/shelves/' + shelf.id) ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"></path>
						</svg>
						<span class="truncate flex-1 min-w-0">{shelf.name}</span>
					</div>
				</a>
			{/each}
		</div>
	</nav>

		<footer class="flex-shrink-0 p-4 pb-[calc(1rem+env(safe-area-inset-bottom))] border-t border-[var(--color-surface-border)] space-y-1 bg-[var(--color-surface-overlay)]">
		<button
			onclick={async () => {
				await fetch('/api/auth/logout', { method: 'POST' });
				window.location.href = '/login';
			}}
			class="w-full flex items-center space-x-3 px-3 py-2 rounded-lg text-[var(--color-surface-text)] hover:bg-red-500/20 hover:text-red-400 transition-colors"
		>
			<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"></path>
			</svg>
			<span>Logout</span>
		</button>
	</footer>
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
			class="block px-3 py-2 text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => { closeLibraryMenu(); $mobileMenuOpen = false; }}
		>
			Open Library
		</a>
		<button
			type="button"
			class="block w-full px-3 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => openEditLibrary(activeLibraryMenu)}
		>
			Edit Library
		</button>
		<button
			type="button"
			class="block w-full px-3 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] disabled:cursor-not-allowed disabled:opacity-60"
			role="menuitem"
			disabled={isLibraryScanActive(activeLibraryMenu)}
			onclick={() => scanLibrary(activeLibraryMenu)}
		>
			{isLibraryScanActive(activeLibraryMenu) ? 'Scan Running' : 'Scan Library'}
		</button>
		<button
			type="button"
			class="block w-full px-3 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => regenerateLibraryCovers(activeLibraryMenu, 'all')}
		>
			Regenerate Covers
		</button>
		<button
			type="button"
			class="block w-full px-3 py-2 text-left text-sm text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)]"
			role="menuitem"
			onclick={() => regenerateLibraryCovers(activeLibraryMenu, 'missing')}
		>
			Regenerate Missing Covers
		</button>
		<a
			href="/settings?tab=general"
			class="block px-3 py-2 text-sm text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-base)] hover:text-[var(--color-surface-text)]"
			role="menuitem"
			onclick={closeLibraryMenu}
		>
			Manage Libraries
		</a>
	</div>
{/if}

	  {#if showLibraryModal}
		<div class="fixed inset-0 bg-black/80 flex items-center justify-center z-[100] p-4" role="dialog" aria-modal="true" tabindex="-1">
			<button
				type="button"
				class="absolute inset-0 z-0"
				aria-label="Close add library modal"
				onclick={closeLibraryModal}
			></button>
			<div class="relative z-10 bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-2xl max-h-[90vh] overflow-hidden shadow-2xl">
			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">{editingLibrary ? 'Edit Library' : 'Add Library'}</h3>
			</div>
			<div class="p-6 space-y-6 overflow-y-auto max-h-[calc(90vh-120px)]">
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="library-name" class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
								Library Name
							</label>
							<input
								id="library-name"
								type="text"
								bind:value={libraryForm.name}
								placeholder="Enter library name"
								class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
							>
						</div>
						<div>
							<div class="mb-2 block text-sm font-medium text-[var(--color-surface-text)]">
								Icon
							</div>
						{#if currentLibraryIcon}
							<div class="flex items-center gap-3 rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-2">
								<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-surface-overlay)] text-[var(--color-primary-400)]">
									{#if currentLibraryIcon.svg}
										<div class="library-icon-preview h-5 w-5">{@html currentLibraryIcon.svg}</div>
									{:else}
										<span class="text-xs font-semibold uppercase">{currentLibraryIcon.name.slice(0, 1) || '?'}</span>
									{/if}
								</div>
								<div class="min-w-0 flex-1">
									<div class="truncate text-sm font-medium text-[var(--color-surface-text)]">{currentLibraryIcon.name}</div>
									<div class="text-xs text-[var(--color-surface-text-muted)]">
										{currentLibraryIcon.source === 'custom' ? 'Custom SVG' : currentLibraryIcon.source === 'svg' ? 'SVG Library' : 'Prime Icons'}
									</div>
								</div>
								<button
									type="button"
									onclick={clearLibraryIcon}
									class="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--color-surface-text-muted)] hover:bg-[var(--color-surface-overlay)] hover:text-[var(--color-surface-text)]"
									title="Remove icon"
									aria-label="Remove icon"
								>
									<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
										<path d="M18 6 6 18"></path>
										<path d="m6 6 12 12"></path>
									</svg>
								</button>
							</div>
						{:else}
							<button
								type="button"
								onclick={openLibraryIconPicker}
								class="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-3 py-3 text-sm font-medium text-[var(--color-primary-400)] hover:border-[var(--color-primary-500)] hover:bg-[var(--color-surface-overlay)]"
							>
								<svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
									<path d="M12 5v14"></path>
									<path d="M5 12h14"></path>
								</svg>
								Select icon
							</button>
						{/if}
					</div>
				</div>

				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3">
					<label for="library-exclude-suggestions" class="flex items-start justify-between gap-4">
						<span class="min-w-0">
							<span class="block text-sm font-medium text-[var(--color-surface-text)]">Exclude from discovery and recommendations</span>
							<span class="mt-1 block text-xs leading-5 text-[var(--color-surface-text-muted)]">
								Keep this library searchable and readable, but hide its books from dashboard discovery and similar book suggestions.
							</span>
						</span>
						<input
							id="library-exclude-suggestions"
							type="checkbox"
							bind:checked={libraryForm.exclude_from_suggestions}
							class="mt-1 h-4 w-4 shrink-0 rounded border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
						>
					</label>
				</div>

				<div class="rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-base)] px-4 py-3">
					<label for="sidebar-library-comic-spread-fallback" class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
						Wide Comic Fallback Cover
					</label>
					<select
						id="sidebar-library-comic-spread-fallback"
						bind:value={libraryForm.comic_spread_fallback}
						class="w-full rounded-lg border border-[var(--color-surface-border)] bg-[var(--color-surface-overlay)] px-3 py-2 text-sm text-[var(--color-surface-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)]"
					>
						{#each inheritedComicSpreadFallbackOptions as option}
							<option value={option.value} class="bg-[var(--color-surface-overlay)] text-[var(--color-surface-text)]">{option.label}</option>
						{/each}
					</select>
				</div>

					<div>
						<div class="flex items-center justify-between mb-2">
							<div class="block text-sm font-medium text-[var(--color-surface-text)]">
								Book Folders
							</div>
							<button
								type="button"
								onclick={openDirectoryModal}
								class="px-3 py-1.5 text-sm bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white rounded-lg transition-colors"
							>
							Add Folder
						</button>
					</div>
					<div class="space-y-2 min-h-[60px] border-2 border-dashed border-[var(--color-surface-border)] rounded-lg p-4">
						{#if libraryForm.paths.length === 0 || (libraryForm.paths.length === 1 && !libraryForm.paths[0].trim())}
							<div class="text-center py-4 text-[var(--color-surface-text-muted)]">
								<svg class="w-8 h-8 mx-auto mb-2 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
								</svg>
								<p class="text-sm">No folders selected</p>
								<p class="text-xs mt-1">Click "Add Folder" to select directories</p>
							</div>
						{:else}
							{#each libraryForm.paths.filter(p => p.trim()) as path}
								<div class="flex items-center justify-between bg-[var(--color-surface-base)] rounded-lg p-3 border border-[var(--color-surface-border)]">
									<div class="flex items-center space-x-3">
										<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
										</svg>
										<span class="font-mono text-sm text-[var(--color-surface-text)]">{path}</span>
									</div>
									<button
										type="button"
										onclick={() => removeLibraryPath(libraryForm.paths.indexOf(path))}
										class="p-1 rounded text-red-400 hover:text-red-300 hover:bg-red-500/10 transition-colors"
										title="Remove folder"
									>
										<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
										</svg>
									</button>
								</div>
							{/each}
						{/if}
					</div>
				</div>

					<div>
						<div class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
							Options
						</div>
					<div class="space-y-3">
					<div class="flex items-center space-x-3">
								<input
									type="checkbox"
								id="auto-scan-sidebar"
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							/>
							<label for="auto-scan-sidebar" class="text-sm text-[var(--color-surface-text)]">
								Automatically scan for new books
							</label>
						</div>
						<div class="flex items-center space-x-3">
							<input
								type="checkbox"
								id="include-subdirs-sidebar"
								checked
								class="rounded border-[var(--color-surface-border)] bg-[var(--color-surface-base)] text-[var(--color-primary-500)] focus:ring-[var(--color-primary-500)]"
							/>
							<label for="include-subdirs-sidebar" class="text-sm text-[var(--color-surface-text)]">
								Include subdirectories
							</label>
						</div>
					</div>
				</div>
				</div>
				<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3">
					<button
						type="button"
						onclick={closeLibraryModal}
						class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
					>
						Cancel
					</button>
					<button
						type="button"
						onclick={saveLibrary}
						disabled={!libraryForm.name.trim() || libraryForm.paths.filter(p => p.trim()).length === 0 || isCreating}
						class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{#if isCreating}
						Saving...
					{:else if editingLibrary}
						Save Library
					{:else}
						Create Library
					{/if}
				</button>
				</div>
			</div>
		</div>

		<LibraryIconPicker
			open={showLibraryIconPicker}
			selectedIcon={libraryForm.icon}
			onSelect={selectLibraryIcon}
			onClose={closeLibraryIconPicker}
		/>

		{#if showDirectoryModal}
			<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-[110] p-4">
				<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-lg max-h-[80vh] flex flex-col overflow-hidden">
				<div class="px-6 py-4 border-b border-[var(--color-surface-border)] flex-shrink-0">
					<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">
						Select Directory
					</h3>
					<p class="text-sm text-[var(--color-surface-text-muted)] mt-1">
						Current: <span class="font-mono">{currentDirectory}</span>
					</p>
				</div>
				<div class="p-6 overflow-y-auto custom-scrollbar flex-1 min-h-0">
					{#if directoryLoading}
						<div class="text-center py-8">
							<div class="mx-auto mb-3 h-8 w-8 animate-spin rounded-full border-2 border-[var(--color-surface-border)] border-t-[var(--color-primary-500)]"></div>
							<p class="text-[var(--color-surface-text-muted)]">Loading directories...</p>
						</div>
					{:else if directoryContents.length === 0}
						<div class="text-center py-8">
							<svg class="w-8 h-8 text-[var(--color-surface-text-muted)] mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
							</svg>
							<p class="text-[var(--color-surface-text-muted)]">No contents available</p>
						</div>
					{:else}
						<div class="space-y-1">
								{#each directoryContents as item}
									<button
										type="button"
										onclick={() => selectDirectory(item)}
										class="w-full flex items-center space-x-3 px-3 py-2 rounded-lg text-left hover:bg-[var(--color-surface-base)] transition-colors {item.type === 'directory' ? 'cursor-pointer' : 'cursor-default'}"
										disabled={item.type !== 'directory'}
								>
									{#if item.type === 'directory'}
										<svg class="w-5 h-5 text-[var(--color-primary-500)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"></path>
										</svg>
									{:else}
										<svg class="w-5 h-5 text-[var(--color-surface-text-muted)]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
											<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"></path>
										</svg>
									{/if}
									<span class="text-[var(--color-surface-text)]">{item.name}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
					<div class="px-6 py-4 border-t border-[var(--color-surface-border)] flex justify-end space-x-3 flex-shrink-0">
						<button
							type="button"
							onclick={closeDirectoryModal}
							class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
						>
							Cancel
						</button>
						<button
							type="button"
							onclick={addSelectedDirectory}
							disabled={!currentDirectory || currentDirectory === '/'}
							class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
					>
						Select Directory
					</button>
				</div>
			</div>
		</div>
	{/if}
  {/if}

<style>
	.library-icon-preview :global(svg) {
		display: block;
		width: 100%;
		height: 100%;
		overflow: visible;
	}
</style>
