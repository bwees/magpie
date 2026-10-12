<script lang="ts">
	import MoviesView from '$lib/movies/MoviesView.svelte';
	import { NAV_ITEMS } from '$lib/navigation/nav';
</script>

<svelte:head>
	<title>{NAV_ITEMS.MOVIES.label} · Magpie</title>
</svelte:head>

<MoviesView />
