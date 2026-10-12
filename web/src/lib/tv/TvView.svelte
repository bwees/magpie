<script lang="ts">
	import { onMount } from 'svelte';
	import BlockState from '$lib/components/BlockState.svelte';
	import FilterChips from '$lib/components/FilterChips.svelte';
	import InlineError from '$lib/components/InlineError.svelte';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { formatSize } from '$lib/search/format';
	import { ProfileOptions } from '$lib/settings/quality/profile-options.svelte';
	import SeriesRowItem from './SeriesRowItem.svelte';
	import { FILTER_LABELS, TV_FILTERS, TvPage } from './tv-page.svelte';

	const page = new TvPage();
	const profiles = new ProfileOptions();
	const live = getLive();

	const SKELETON_ROWS = 6;

	onMount(() => {
		void page.load();
		void profiles.load();
		return page.watch(live);
	});
</script>

<div class="mb-5 flex flex-wrap items-baseline gap-3">
	<h1 class="text-2xl font-semibold tracking-tight">TV</h1>
	{#if page.series.ready}
		<span class="text-sm text-muted-foreground tabular-nums">
			{page.rows.length} series{page.coverage.ready ? ` · ${formatSize(page.totalSize)}` : ''}
		</span>
	{/if}
</div>

<div class="mb-5">
	<FilterChips
		filters={TV_FILTERS}
		selected={page.filter}
		labels={FILTER_LABELS}
		count={(filter) => page.count(filter)}
		onSelect={(filter) => page.select(filter)}
	/>
</div>

{#if page.series.ready && page.coverage.error}
	<div class="mb-4">
		<InlineError
			label="Episode coverage"
			message={page.coverage.error}
			onRetry={() => page.coverage.load()}
		/>
	</div>
{/if}

<BlockState
	block={page.series}
	label="series"
	empty="No series yet. Search above to add one."
	onRetry={() => page.load()}
>
	{#snippet skeleton()}
		<div class="flex flex-col gap-2">
			{#each { length: SKELETON_ROWS }, i (i)}
				<Skeleton class="h-16 w-full rounded-lg" />
			{/each}
		</div>
	{/snippet}
	{#if page.shown.length === 0}
		<p class="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
			No series match this filter.
		</p>
	{:else}
		<ul class="divide-y overflow-hidden rounded-xl border">
			{#each page.shown as row (row.title.id)}
				<li>
					<SeriesRowItem
						{row}
						profileName={profiles.name(row.title.profileId)}
						coverageLoading={page.coverage.status === LoadStatus.LOADING}
					/>
				</li>
			{/each}
		</ul>
	{/if}
</BlockState>
