<script lang="ts">
	import DownloadsSettings from '$lib/settings/downloads/DownloadsSettings.svelte';
	import SearchOnAddCard from '$lib/settings/downloads/SearchOnAddCard.svelte';
</script>

<svelte:head>
	<title>Downloads · Settings · Magpie</title>
</svelte:head>

<div class="mx-auto grid max-w-4xl gap-6">
	<DownloadsSettings />
	<SearchOnAddCard />
</div>
