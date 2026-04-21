<script lang="ts">
	let {
		src = '/branding/logo.jpg',
		alt = 'Cryptorum logo',
		sizeClass = 'h-10 w-10',
		class: className = '',
		roundedClass = 'rounded-xl'
	} = $props<{
		src?: string;
		alt?: string;
		sizeClass?: string;
		class?: string;
		roundedClass?: string;
	}>();
</script>

<img
	src={src}
	alt={alt}
	class={`${sizeClass} ${roundedClass} object-cover shadow-sm ${className}`.trim()}
	loading="eager"
/>
