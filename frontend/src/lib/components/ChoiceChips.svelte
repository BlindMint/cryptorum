<script lang="ts">
	let { label, value, options, onchange }: {
		label: string;
		value: string;
		options: { id: string; name: string }[];
		onchange: (value: string) => void;
	} = $props();
</script>

<div class="chips" role="group" aria-label={label}>
	{#each options as option}
		<button type="button" aria-pressed={value === option.id} onclick={() => onchange(option.id)}>{option.name}</button>
	{/each}
</div>

<style>
	.chips { display: flex; flex-wrap: wrap; gap: 4px; }
	button { flex: 1 1 auto; padding: 8px 12px; border: 1px solid var(--color-surface-border); border-radius: 6px; background: transparent; color: var(--color-surface-text); font-size: 12px; cursor: pointer; }
	button:hover { border-color: var(--color-primary-500); }
	button[aria-pressed="true"] { border-color: var(--color-primary-500); background: color-mix(in srgb, var(--color-primary-500) 12%, transparent); }
	button:focus-visible { outline: 2px solid var(--color-primary-500); outline-offset: 2px; }
</style>
