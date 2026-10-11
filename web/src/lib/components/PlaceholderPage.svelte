<script lang="ts">
	import type { NavItem } from '$lib/navigation/nav';

	let { item }: { item: NavItem } = $props();
</script>

<svelte:head>
	<title>{item.label} · Magpie</title>
</svelte:head>

<section
	class="flex min-h-64 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-8 text-center"
>
	<item.icon class="size-8 text-muted-foreground" />
	<h2 class="text-lg font-semibold">{item.label}</h2>
	<p class="text-sm text-muted-foreground">Nothing here yet.</p>
</section>
