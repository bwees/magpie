<script lang="ts">
	import { NAV_ITEMS } from '$lib/navigation/nav';
	import TvView from '$lib/tv/TvView.svelte';
</script>

<svelte:head>
	<title>{NAV_ITEMS.TV.label} · Magpie</title>
</svelte:head>

<TvView />
