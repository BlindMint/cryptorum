<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import ShelfModal from '$lib/components/ShelfModal.svelte';

	let modalOpen = $state(false);
	let initialMagic = $state(false);

	onMount(() => {
		initialMagic = $page.url.searchParams.get('magic') === 'true';
		modalOpen = true;
	});

	function closeModal() {
		modalOpen = false;
		goto('/shelves');
	}
</script>

<div class="min-h-[50vh]"></div>

<ShelfModal
	open={modalOpen}
	mode="create"
	initialMagic={initialMagic}
	onClose={closeModal}
	onSaved={() => goto('/shelves')}
/>
