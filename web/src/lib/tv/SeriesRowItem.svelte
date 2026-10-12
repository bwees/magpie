<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { airDateLabel, episodeCode } from '$lib/catalog/episodes';
	import { ImageWidth, sizedImage, titlePath } from '$lib/catalog/images';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import { formatSize } from '$lib/search/format';
	import { cn } from '$lib/utils';
	import {
		SERIES_STATUS_LABELS,
		SeriesStatus,
		missingEpisodes,
		type SeriesRow
	} from './tv-page.svelte';

	let {
		row,
		profileName,
		coverageLoading
	}: { row: SeriesRow; profileName?: string; coverageLoading: boolean } = $props();

	const NO_VALUE = '—';
	const FULL_PERCENT = 100;

	const title = $derived(row.title);
	const have = $derived(row.coverage?.episodesWithFile ?? 0);
	const total = $derived(title.wantedEpisodeCount);
	const missing = $derived(missingEpisodes(row));
	const percent = $derived(total > 0 ? Math.min((have / total) * FULL_PERCENT, FULL_PERCENT) : 0);
	const meta = $derived(
		[
			title.year || undefined,
			`${title.seasonCount} season${title.seasonCount === 1 ? '' : 's'}`,
			profileName
		]
			.filter(Boolean)
			.join(' · ')
	);
	const next = $derived(
		title.nextEpisode
			? `${episodeCode(title.nextEpisode.season, title.nextEpisode.number)} · ${airDateLabel(title.nextEpisode.airDate)}`
			: NO_VALUE
	);
</script>

{#snippet coverage(compact: boolean)}
	{#if coverageLoading}
		<Skeleton class="h-3 w-full" />
	{:else if row.coverage}
		<div class={cn('mb-1 flex justify-between gap-2', compact ? 'text-[11px]' : 'text-xs')}>
			<span class="tabular-nums">{have} / {total} episodes</span>
			{#if total > 0}
				<span class={missing > 0 ? 'text-amber-500' : 'text-emerald-500'}>
					{missing > 0 ? `${missing} missing` : 'Complete'}
				</span>
			{/if}
		</div>
		<div class="h-1.5 overflow-hidden rounded-full bg-muted">
			<div
				class={cn('h-full rounded-full', missing > 0 ? 'bg-amber-500' : 'bg-emerald-500')}
				style:width="{percent}%"
			></div>
		</div>
	{:else}
		<span class="text-xs text-muted-foreground">{NO_VALUE}</span>
	{/if}
{/snippet}

<a
	href={titlePath(title.id)}
	class="flex w-full items-center gap-4 px-4 py-3 text-left transition-colors hover:bg-muted/40"
>
	<PosterImage
		src={sizedImage(title.images.poster, ImageWidth.THUMB)}
		alt=""
		class="w-10 shrink-0"
	/>
	<div class="min-w-0 flex-1 md:w-56 md:flex-none">
		<div class="truncate font-medium">{title.name}</div>
		<div class="truncate text-xs text-muted-foreground">{meta}</div>
		<div class="mt-1.5 md:hidden">{@render coverage(true)}</div>
	</div>
	<Badge
		variant={title.seriesStatus === SeriesStatus.CONTINUING ? 'default' : 'secondary'}
		class="hidden w-24 justify-center md:inline-flex"
	>
		{SERIES_STATUS_LABELS[title.seriesStatus] ?? NO_VALUE}
	</Badge>
	<div class="hidden min-w-0 flex-1 md:block">{@render coverage(false)}</div>
	<div class="hidden w-40 text-right text-xs text-muted-foreground lg:block">{next}</div>
	<div class="hidden w-20 text-right text-sm tabular-nums md:block">
		{row.coverage ? formatSize(row.coverage.sizeBytes) : NO_VALUE}
	</div>
</a>
