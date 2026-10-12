<script lang="ts">
	import { onMount } from 'svelte';
	import LayoutGrid from '@lucide/svelte/icons/layout-grid';
	import List from '@lucide/svelte/icons/list';
	import BlockState from '$lib/components/BlockState.svelte';
	import FilterChips from '$lib/components/FilterChips.svelte';
	import InlineError from '$lib/components/InlineError.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import { STAGE_LABELS, Stage } from '$lib/activity/pipeline';
	import { ImageWidth, sizedImage, titlePath } from '$lib/catalog/images';
	import { POSTER_GRID } from '$lib/catalog/layout';
	import type { TitleSort } from '$lib/catalog/library-titles';
	import PosterCard from '$lib/catalog/PosterCard.svelte';
	import PosterGridSkeleton from '$lib/catalog/PosterGridSkeleton.svelte';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import { videoSummary } from '$lib/library/library';
	import { getLive } from '$lib/live/live-client.svelte';
	import { formatSize } from '$lib/search/format';
	import { ProfileOptions } from '$lib/settings/quality/profile-options.svelte';
	import { cn } from '$lib/utils';
	import {
		FILTER_LABELS,
		MOVIE_FILTERS,
		MOVIE_SORTS,
		MoviesLayout,
		MoviesPage,
		SORT_LABELS,
		type MovieRow
	} from './movies-page.svelte';

	const page = new MoviesPage();
	const profiles = new ProfileOptions();
	const live = getLive();

	const TABLE_COLUMNS = ['', 'Title', 'Profile', 'Status', 'Video', 'Size'];
	const SKELETON_ROWS = 6;
	const NO_VALUE = '—';

	onMount(() => {
		void page.load();
		void profiles.load();
		return page.watch(live);
	});

	function status(row: MovieRow): string {
		const stage = row.stage?.stage ?? '';
		return STAGE_LABELS[stage as Stage] ?? stage;
	}

	function statusVariant(row: MovieRow): 'destructive' | 'secondary' | 'default' {
		if (row.stage?.failed) return 'destructive';
		return row.stage?.stage === Stage.AVAILABLE ? 'secondary' : 'default';
	}

	const layoutButton = 'p-1.5 transition-colors';
</script>

<div class="mb-5 flex flex-wrap items-center gap-3">
	<h1 class="text-2xl font-semibold tracking-tight">Movies</h1>
	{#if page.titles.ready}
		<span class="text-sm text-muted-foreground tabular-nums">{page.rows.length} movies</span>
	{/if}
	<div class="ml-auto flex items-center gap-2">
		<Select.Root
			type="single"
			value={page.sort}
			onValueChange={(value) => void page.sortBy(value as TitleSort)}
		>
			<Select.Trigger size="sm" class="w-40" aria-label="Sort"
				>{SORT_LABELS[page.sort]}</Select.Trigger
			>
			<Select.Content>
				{#each MOVIE_SORTS as sort (sort)}
					<Select.Item value={sort} label={SORT_LABELS[sort]} />
				{/each}
			</Select.Content>
		</Select.Root>
		<div class="hidden overflow-hidden rounded-md border md:flex" role="group" aria-label="Layout">
			<button
				type="button"
				class={cn(layoutButton, page.layout === MoviesLayout.GRID && 'bg-muted')}
				aria-pressed={page.layout === MoviesLayout.GRID}
				aria-label="Grid"
				onclick={() => (page.layout = MoviesLayout.GRID)}
			>
				<LayoutGrid class="size-4" />
			</button>
			<button
				type="button"
				class={cn(layoutButton, page.layout === MoviesLayout.TABLE && 'bg-muted')}
				aria-pressed={page.layout === MoviesLayout.TABLE}
				aria-label="Table"
				onclick={() => (page.layout = MoviesLayout.TABLE)}
			>
				<List class="size-4" />
			</button>
		</div>
	</div>
</div>

<div class="mb-5">
	<FilterChips
		filters={MOVIE_FILTERS}
		selected={page.filter}
		labels={FILTER_LABELS}
		count={(filter) => page.count(filter)}
		onSelect={(filter) => page.select(filter)}
	/>
</div>

{#if page.titles.ready && (page.stages.error || page.files.error)}
	<div class="mb-4 flex flex-col gap-2">
		{#if page.stages.error}
			<InlineError
				label="Pipeline stages"
				message={page.stages.error}
				onRetry={() => page.stages.load()}
			/>
		{/if}
		{#if page.files.error}
			<InlineError label="Files" message={page.files.error} onRetry={() => page.files.load()} />
		{/if}
	</div>
{/if}

{#snippet grid()}
	<div class={POSTER_GRID}>
		{#each page.shown as row (row.title.id)}
			<PosterCard
				titleId={row.title.id}
				name={row.title.name}
				year={row.title.year}
				poster={row.title.images.poster}
				stage={row.stage}
			/>
		{/each}
	</div>
{/snippet}

<BlockState
	block={page.titles}
	label="movies"
	empty="No movies yet. Search above to add one."
	onRetry={() => page.load()}
>
	{#snippet skeleton()}
		{#if page.layout === MoviesLayout.TABLE}
			<div class="hidden flex-col gap-2 md:flex">
				{#each { length: SKELETON_ROWS }, i (i)}
					<Skeleton class="h-12 w-full rounded-lg" />
				{/each}
			</div>
			<div class="md:hidden"><PosterGridSkeleton /></div>
		{:else}
			<PosterGridSkeleton />
		{/if}
	{/snippet}
	{#if page.shown.length === 0}
		<p class="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
			No movies match this filter.
		</p>
	{:else if page.layout === MoviesLayout.GRID}
		{@render grid()}
	{:else}
		<div class="md:hidden">{@render grid()}</div>
		<div class="hidden rounded-xl border md:block">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						{#each TABLE_COLUMNS as column, i (i)}
							<Table.Head
								class={cn(i === 0 && 'w-12', i === TABLE_COLUMNS.length - 1 && 'text-right')}
							>
								{column}
							</Table.Head>
						{/each}
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each page.shown as row (row.title.id)}
						{@const file = row.files[0]}
						<Table.Row class="relative">
							<Table.Cell>
								<PosterImage
									src={sizedImage(row.title.images.poster, ImageWidth.THUMB)}
									alt=""
									class="w-8 rounded-sm"
								/>
							</Table.Cell>
							<Table.Cell class="font-medium">
								<a
									href={titlePath(row.title.id)}
									class="after:absolute after:inset-0 hover:underline"
								>
									{row.title.name}
								</a>
								<span class="font-normal text-muted-foreground">({row.title.year})</span>
							</Table.Cell>
							<Table.Cell>{profiles.name(row.title.profileId) ?? NO_VALUE}</Table.Cell>
							<Table.Cell>
								{#if row.stage}
									<Badge variant={statusVariant(row)}>{status(row)}</Badge>
								{:else}
									<span class="text-muted-foreground">{NO_VALUE}</span>
								{/if}
							</Table.Cell>
							<Table.Cell class="text-xs text-muted-foreground">
								{file ? videoSummary(file) : NO_VALUE}
							</Table.Cell>
							<Table.Cell class="text-right tabular-nums">
								{file ? formatSize(file.size) : NO_VALUE}
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</div>
	{/if}
</BlockState>
