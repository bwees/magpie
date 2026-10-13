<script lang="ts">
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { Button } from '$lib/components/ui/button';

	let { error, reset }: { error: unknown; reset: () => void } = $props();

	const failedAt = page.url.pathname;

	$effect(() => {
		if (page.url.pathname !== failedAt) {
			untrack(reset);
		}
	});
</script>

<section
	class="flex min-h-64 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-8 text-center"
	role="alert"
>
	<TriangleAlert class="size-8 text-muted-foreground" />
	<h2 class="text-lg font-semibold">This page hit an error</h2>
	{#if error instanceof Error}
		<p class="text-sm text-muted-foreground">{error.message}</p>
	{/if}
	<Button variant="secondary" onclick={reset}>Retry</Button>
</section>
