<script lang="ts">
	import Film from '@lucide/svelte/icons/film';
	import { cn } from '$lib/utils';

	let {
		src,
		alt,
		class: className = ''
	}: { src: string | null | undefined; alt: string; class?: string } = $props();

	let failed = $state(false);
</script>

<div class={cn('aspect-[2/3] overflow-hidden rounded-md bg-muted shadow-sm', className)}>
	{#if src && !failed}
		<img
			{src}
			{alt}
			loading="lazy"
			class="size-full object-cover"
			onerror={() => (failed = true)}
		/>
	{:else}
		<div class="flex size-full items-center justify-center text-muted-foreground" aria-label={alt}>
			<Film class="size-1/3 max-h-8 max-w-8" />
		</div>
	{/if}
</div>
