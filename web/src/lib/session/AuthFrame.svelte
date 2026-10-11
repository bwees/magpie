<script lang="ts">
	import type { Snippet } from 'svelte';
	import Bird from '@lucide/svelte/icons/bird';
	import { cn } from '$lib/utils';

	let {
		title,
		wide = false,
		children
	}: { title: string; wide?: boolean; children: Snippet } = $props();
</script>

<svelte:head>
	<title>{title} · Magpie</title>
</svelte:head>

<div class="flex min-h-svh flex-col items-center px-4 py-8 md:justify-center md:py-12">
	<div class={cn('flex w-full flex-col gap-6', wide ? 'max-w-4xl' : 'max-w-sm')}>
		<div class="flex items-center gap-2 self-center">
			<span
				class="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"
			>
				<Bird class="size-4" />
			</span>
			<span class="text-lg font-semibold">Magpie</span>
		</div>
		{@render children()}
	</div>
</div>
