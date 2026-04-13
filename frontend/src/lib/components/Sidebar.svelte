<script lang="ts">
  	import { onMount, tick } from 'svelte';
  	import { page } from '$app/stores';
  	import { mobileMenuOpen } from '$lib/stores';

 	interface Library {
 		id: number;
 		name: string;
 		icon: string;
 		book_count: number;
 	}

 	interface Shelf {
 		id: number;
 		name: string;
 		icon: string;
 	}

  	let libraries = $state<Library[]>([]);
  	let shelves = $state<Shelf[]>([]);
  	let isLoading = $state(false);
  	let isCreating = $state(false);
  	let scanIntervals = $state<Map<number, number>>(new Map());

  	// Library modal state
   	let showLibraryModal = $state(false);
   	let libraryForm = $state({
   		name: '',
   		icon: '',
   		paths: ['']
   	});
   	let lastRefresh = 0;
   	let modalPortal: HTMLDivElement;

   	// Directory browser state
   	let showDirectoryModal = $state(false);
   	let currentDirectory = $state('/');
   	let directoryContents = $state<any[]>([]);

  	// Helper function to check if a navigation item is active
  	function isActive(href: string): boolean {
  		const currentPath = $page.url.pathname;
  		const currentSearch = $page.url.search;

  		if (href === '/') {
  			return currentPath === '/' && !currentSearch.includes('library=');
  		}

  		if (href === '/library') {
  			return currentPath === '/library' && !currentSearch.includes('library=');
  		}

  		if (href.startsWith('/library?library=')) {
  			return currentSearch.includes(href.split('?')[1]);
  		}

  		if (href.startsWith('/shelves/')) {
  			return currentPath.startsWith('/shelves/') && currentPath === href;
  		}

  		return currentPath === href;
  	}

  	onMount(async () => {
    		try {
    			const res = await fetch('/api/libraries');
    			if (res.ok) libraries = await res.json();
    		} catch (e) {
    			console.error('Failed to fetch libraries:', e);
    		}

    		(window as any).refreshSidebar = loadData;
    	});

  	function openLibraryModal() {
  		libraryForm = { name: '', icon: '', paths: [''] };
  		showLibraryModal = true;
  	}

  	function closeLibraryModal() {
  		showLibraryModal = false;
  	}

  	function removeLibraryPath(index: number) {
  		if (libraryForm.paths.length > 1) {
  			libraryForm.paths.splice(index, 1);
  			libraryForm.paths = [...libraryForm.paths];
  		}
  	}

  	async function createLibrary() {
  		if (!libraryForm.name.trim()) return;

  		isCreating = true;
  		try {
  			const filteredPaths = libraryForm.paths.filter(p => p.trim());
  			const response = await fetch('/api/libraries', {
  				method: 'POST',
  				headers: { 'Content-Type': 'application/json' },
  				body: JSON.stringify({ ...libraryForm, paths: filteredPaths })
  			});

  			if (response.ok) {
  				const newLibrary = await response.json();
  				closeLibraryModal();

  				// Reload data to get is_importing state and updated book_count
  				await loadData();

  				// Trigger a scan for the new library
  				await scanLibrary(newLibrary);
  			} else {
  				console.error('Failed to create library');
  			}
  		} catch (error) {
  			console.error('Error creating library:', error);
  		} finally {
  			isCreating = false;
  		}
  	}

  	async function scanLibrary(library: any) {
  		try {
  			const response = await fetch(`/api/libraries/${library.id}/scan`, { method: 'POST' });
  			if (response.ok) {
  				// Start polling for updates
  				const interval = setInterval(async () => {
  					await loadData();
  					// Check if still importing
  					const libsRes = await fetch('/api/libraries');
  					if (libsRes.ok) {
  						const libs = await libsRes.json();
  						const updatedLib = libs.find((l: any) => l.id === library.id);
  						if (updatedLib && !updatedLib.is_importing) {
  							clearInterval(interval);
  							scanIntervals.delete(library.id);
  						}
  					}
  				}, 3000);

  				scanIntervals.set(library.id, interval as any);

  				// Stop after 5 minutes
  				setTimeout(() => {
  					clearInterval(interval);
  					scanIntervals.delete(library.id);
  				}, 300000);
  			}
  		} catch (e) {
  			console.error('Failed to scan library:', e);
  		}
  	}

  	async function openDirectoryModal() {
  		showDirectoryModal = true;
  		try {
  			const response = await fetch('/api/directories?path=/books');
  			if (response.ok) {
  				loadDirectoryContents('/books');
  			} else {
  				loadDirectoryContents('/');
  			}
  		} catch (e) {
  			loadDirectoryContents('/');
  		}
  	}

  	function closeDirectoryModal() {
  		showDirectoryModal = false;
  	}

  	async function loadDirectoryContents(path: string) {
  		currentDirectory = path;
  		try {
  			const response = await fetch(`/api/directories?path=${encodeURIComponent(path)}`);
  			if (response.ok) {
  				directoryContents = await response.json();
  			} else {
  				directoryContents = [];
  			}
  		} catch (e) {
  			directoryContents = [];
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
 				fetch('/api/libraries'),
 				fetch('/api/shelves')
 			]);

 			if (libsRes.ok) {
 				const libs = await libsRes.json();
 				if (libs) libraries = libs;
 			}

 			if (shelvesRes.ok) {
 				const sh = await shelvesRes.json();
 				if (sh) shelves = sh;
 			}
 		} catch (e) {
 			console.error('Failed to load navigation data:', e);
 		} finally {
 			isLoading = false;
 		}
	}


</script>



<aside
	class="
		fixed lg:static top-16 lg:top-0 bottom-0 left-0 z-40
		w-64 max-w-[400px] min-w-[240px] bg-[var(--color-surface-overlay)] border-r border-[var(--color-surface-border)]
		transform transition-transform duration-200 ease-in-out
		lg:translate-x-0 lg:resize-x overflow-hidden
		{$mobileMenuOpen ? 'translate-x-0' : '-translate-x-full'}
		flex flex-col h-full
	"
	style="resize: horizontal;"
>
  	<div class="flex-shrink-0 p-4 pb-3 space-y-1">
		<a
			href="/"
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"></path>
			</svg>
			<span>Dashboard</span>
		</a>

		<a
			href="/library"
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/library') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/library') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
			</svg>
			<span>All Books</span>
		</a>

		<a
			href="/authors"
			class="flex items-center space-x-3 px-3 py-2 rounded-lg transition-all duration-200 {isActive('/authors') ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
		>
			<svg class="w-5 h-5 transition-transform duration-200 {isActive('/authors') ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
			</svg>
			<span>Authors</span>
		</a>

		<a
			href="/series"
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

			{#each libraries as library}
   				<a
   					href="/library?library={library.id}"
   					onclick={() => $mobileMenuOpen = false}
   					class="flex items-center px-3 py-2 rounded-lg transition-all duration-200 {isActive('/library?library=' + library.id) ? 'bg-[var(--color-primary-500)]/20 text-[var(--color-primary-500)] shadow-sm' : 'text-[var(--color-surface-text)] hover:bg-[var(--color-surface-base)] hover:translate-x-1 hover:shadow-sm'}"
   				>
					<div class="flex items-center space-x-3 flex-1 min-w-0">
						{#if 'is_importing' in library && library.is_importing}
							<svg class="animate-spin w-5 h-5 text-[var(--color-primary-500)] flex-shrink-0 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
							</svg>
						{:else}
							<svg class="w-5 h-5 text-[var(--color-primary-500)] flex-shrink-0 transition-transform duration-200 {isActive('/library?library=' + library.id) ? '' : 'group-hover:scale-110'}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253"></path>
							</svg>
						{/if}
  						<span class="truncate flex-1 min-w-0">{library.name}</span>
  					</div>
  					<span class="text-xs text-[var(--color-surface-500)] font-medium px-2 py-0.5 bg-[var(--color-surface-700)] rounded-md ml-2 flex-shrink-0">{library.book_count}</span>
  				</a>
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
 					onclick={() => $mobileMenuOpen = false}
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
  					onclick={() => $mobileMenuOpen = false}
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

	<footer class="flex-shrink-0 p-4 border-t border-[var(--color-surface-border)] space-y-1 bg-[var(--color-surface-overlay)]">
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
</aside>

  {#if showLibraryModal}
  	<div class="fixed inset-0 bg-black/80 flex items-center justify-center z-[100] p-4" onclick={closeLibraryModal} role="dialog" aria-modal="true">
  		<div class="bg-[var(--color-surface-overlay)] rounded-lg border border-[var(--color-surface-border)] w-full max-w-2xl max-h-[90vh] overflow-hidden shadow-2xl" onclick={(e) => e.stopPropagation()}>
  			<div class="px-6 py-4 border-b border-[var(--color-surface-border)]">
  				<h3 class="text-lg font-semibold text-[var(--color-surface-text)]">Add Library</h3>
  			</div>
  			<div class="p-6 space-y-6 overflow-y-auto max-h-[calc(90vh-120px)]">
  				<div class="grid grid-cols-2 gap-4">
  					<div>
  						<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
  							Library Name
  						</label>
  						<input
  							type="text"
  							bind:value={libraryForm.name}
  							placeholder="Enter library name"
  							class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
  						>
  					</div>
  					<div>
  						<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
  							Icon
  						</label>
  						<input
  							type="text"
  							bind:value={libraryForm.icon}
  							placeholder="Icon identifier"
  							class="w-full px-3 py-2 rounded-lg bg-[var(--color-surface-base)] border border-[var(--color-surface-border)] text-[var(--color-surface-text)] placeholder-[var(--color-surface-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary-500)] focus:border-transparent"
  						>
  					</div>
  				</div>

  				<div>
  					<div class="flex items-center justify-between mb-2">
  						<label class="block text-sm font-medium text-[var(--color-surface-text)]">
  							Book Folders
  						</label>
  						<button
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
  					<label class="block text-sm font-medium text-[var(--color-surface-text)] mb-2">
  						Options
  					</label>
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
  					onclick={closeLibraryModal}
  					class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
  				>
  					Cancel
  				</button>
   				<button
   					onclick={createLibrary}
   					disabled={!libraryForm.name.trim() || libraryForm.paths.filter(p => p.trim()).length === 0 || isCreating}
   					class="px-4 py-2 rounded-lg bg-[var(--color-primary-500)] hover:bg-[var(--color-primary-600)] text-white font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
   				>
   					{#if isCreating}
   						Creating...
   					{:else}
   						Create Library
   					{/if}
   				</button>
  			</div>
  		</div>
  	</div>

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
  					{#if directoryContents.length === 0}
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
  						onclick={closeDirectoryModal}
  						class="px-4 py-2 rounded-lg text-[var(--color-surface-text-muted)] hover:text-[var(--color-surface-text)] transition-colors"
  					>
  						Cancel
  					</button>
  					<button
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
