<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { apiErrorMessage, getAttribution, type AttributionResponse } from '$lib/sdk';

	let attribution = $state<AttributionResponse | undefined>();
	let error = $state('');

	async function load() {
		error = '';
		try {
			attribution = await getAttribution();
		} catch (e) {
			error = apiErrorMessage(e);
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>About · Magpie</title>
</svelte:head>

<div class="mx-auto flex max-w-xl flex-col gap-6">
	<section>
		<h2 class="text-lg font-semibold">Magpie</h2>
		<p class="text-sm text-muted-foreground">
			Finds, downloads, organises, subtitles and transcodes movies and TV for Jellyfin.
		</p>
	</section>

	<section class="flex flex-col gap-3 rounded-xl border p-5" aria-label="Metadata attribution">
		<h3 class="text-sm font-semibold">Metadata and images</h3>
		{#if error}
			<p class="text-sm text-muted-foreground" role="alert">
				{error}
				<Button variant="link" size="xs" class="h-auto p-0" onclick={load}>Retry</Button>
			</p>
		{:else if !attribution}
			<Skeleton class="h-6 w-48" />
			<Skeleton class="h-4 w-full" />
		{:else}
			<a href={attribution.link} target="_blank" rel="noopener noreferrer" class="w-fit">
				<img src={attribution.logoUrl} alt="TMDB" class="h-5" />
			</a>
			<p class="text-sm text-muted-foreground">{attribution.notice}</p>
		{/if}
	</section>
</div>
