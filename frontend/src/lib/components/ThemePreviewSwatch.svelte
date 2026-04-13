<script lang="ts">
	let {
		background,
		foreground,
		sizeClass = 'h-10 w-10'
	} = $props<{
		background: string;
		foreground: string;
		sizeClass?: string;
	}>();
</script>

<div
	class={`inline-flex items-center justify-center rounded-full border border-[currentColor]/30 font-semibold leading-none select-none ${sizeClass}`}
	style={`background-color: ${background}; color: ${foreground}; font-size: 0.95rem; transform: translateY(1px);`}
	aria-hidden="true"
>
	a
</div>
