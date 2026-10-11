<script lang="ts">
	import { page } from '$app/state';
	import Construction from '@lucide/svelte/icons/construction';
	import { sectionFor } from '$lib/settings/sections';

	const current = $derived(sectionFor(page.url.pathname));
</script>

<section
	class="flex min-h-48 flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-8 text-center"
>
	<Construction class="size-6 text-muted-foreground" />
	{#if current}
		<p class="text-sm text-muted-foreground">{current.label} settings are coming soon.</p>
	{:else}
		<p class="text-sm text-muted-foreground">There is no settings section here.</p>
	{/if}
</section>
