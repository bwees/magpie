<script lang="ts">
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import { SearchLayout } from './layout';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { cn } from '$lib/utils';
	import { ImageWidth, sizedImage, titlePath } from './images';
	import PosterImage from './PosterImage.svelte';
	import { LoadState, type TitleSearch } from './title-search.svelte';

	let { search, layout }: { search: TitleSearch; layout: SearchLayout } = $props();

	const GRID_COLUMNS: Record<SearchLayout, string> = {
		[SearchLayout.POPOVER]: 'grid-cols-5',
		[SearchLayout.OVERLAY]: 'grid-cols-3'
	};

	const KIND_LABELS: Record<string, string> = {
		MOVIE: 'Movie',
		SERIES: 'TV'
	};

	const sectionHeading =
		'px-1 pb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase';
</script>

<div class="flex flex-col gap-4">
	{#if search.hasQuery && search.libraryHits.length > 0}
		<section aria-label="In your library">
			<h3 class={sectionHeading}>In your library</h3>
			<ul class="flex flex-col">
				{#each search.libraryHits as hit (hit.id)}
					<li>
						<a
							href={titlePath(hit.id)}
							class="flex items-center gap-3 rounded-md px-1 py-1.5 hover:bg-muted"
							onclick={() => search.close()}
						>
							<PosterImage
								src={sizedImage(hit.images.poster, ImageWidth.THUMB)}
								alt=""
								class="w-8 shrink-0 rounded-sm"
							/>
							<span class="min-w-0 truncate text-sm">{hit.name}</span>
							{#if hit.year}
								<span class="text-sm text-muted-foreground">{hit.year}</span>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<section aria-label={search.hasQuery ? 'From TMDB' : 'Trending this week'}>
		<h3 class={sectionHeading}>{search.hasQuery ? 'From TMDB' : 'Trending this week'}</h3>
		{#if search.tmdbState === LoadState.FAILED}
			<p class="flex items-center gap-2 px-1 text-sm text-muted-foreground" role="alert">
				TMDB unavailable,
				<Button variant="link" size="xs" class="h-auto p-0" onclick={() => search.retry()}>
					retry
				</Button>
			</p>
		{:else if search.tmdbState === LoadState.LOADING || search.tmdbState === LoadState.IDLE}
			<p class="flex items-center gap-2 px-1 text-sm text-muted-foreground" role="status">
				<LoaderCircle class="size-4 animate-spin" />
				Searching TMDB…
			</p>
		{:else if search.tmdbItems.length === 0}
			<p class="px-1 text-sm text-muted-foreground">Nothing on TMDB matches.</p>
		{:else}
			<ul class={cn('grid gap-3', GRID_COLUMNS[layout])}>
				{#each search.tmdbItems as item (`${item.kind}-${item.tmdbId}`)}
					<li class="flex min-w-0 flex-col gap-1">
						<PosterImage src={item.posterUrl} alt={item.name} />
						<span class="truncate text-xs font-medium" title={item.name}>{item.name}</span>
						<span class="text-[11px] text-muted-foreground">
							{item.year || '—'} · {KIND_LABELS[item.kind] ?? item.kind}
						</span>
						{#if item.inLibrary && item.titleId}
							<Badge
								href={titlePath(item.titleId)}
								variant="secondary"
								class="w-full"
								onclick={() => search.close()}
							>
								In library
							</Badge>
						{:else}
							<Button size="xs" class="w-full" disabled title="Adding titles arrives with Requests">
								Add
							</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>
