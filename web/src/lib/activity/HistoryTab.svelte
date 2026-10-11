<script lang="ts">
	import { onMount } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { cn } from '$lib/utils';
	import { LoadStatus, SDK_SOURCE } from './activity-source';
	import { ActivityHistory, HistoryFilter } from './activity-history.svelte';
	import { formatWhen, isAlert } from './format';

	const SKELETON_ROWS = 8;
	const TITLE_PATH = '/titles/';
	const FILTER_LABELS: Record<HistoryFilter, string> = {
		[HistoryFilter.ALL]: 'All',
		[HistoryFilter.PROBLEMS]: 'Problems'
	};

	const live = getLive();
	const history = new ActivityHistory(SDK_SOURCE);
	const feed = $derived(history.feed);

	onMount(() => {
		void history.feed.load();
		return history.connect(live);
	});
</script>

<section class="flex flex-col gap-3" aria-label="History">
	<div class="flex items-center justify-between gap-3">
		<p class="text-sm text-muted-foreground">The last 90 days.</p>
		<div class="flex gap-1" role="group" aria-label="Filter">
			{#each Object.values(HistoryFilter) as filter (filter)}
				<Button
					size="sm"
					variant={history.filter === filter ? 'secondary' : 'ghost'}
					aria-pressed={history.filter === filter}
					onclick={() => history.setFilter(filter)}
				>
					{FILTER_LABELS[filter]}
				</Button>
			{/each}
		</div>
	</div>

	{#if feed.status === LoadStatus.LOADING}
		<div class="flex flex-col gap-2">
			{#each { length: SKELETON_ROWS }, index (index)}
				<Skeleton class="h-10 w-full" />
			{/each}
		</div>
	{:else if feed.status === LoadStatus.ERROR}
		<div class="flex items-center gap-3 text-sm text-destructive" role="alert">
			<span>Couldn't load the history. {feed.error}</span>
			<Button size="sm" variant="outline" onclick={() => feed.retry()}>Retry</Button>
		</div>
	{:else if feed.empty}
		<p class="py-8 text-center text-sm text-muted-foreground">Nothing has happened yet.</p>
	{:else}
		<ol class="divide-y rounded-lg border">
			{#each feed.entries as entry (entry.eventId)}
				{@const alert = isAlert(entry.severity)}
				<li
					class={cn(
						'grid grid-cols-[1fr_auto] gap-x-4 gap-y-0.5 px-3 py-2 text-sm md:grid-cols-[9rem_14rem_1fr]',
						alert && 'bg-destructive/10'
					)}
				>
					<time
						class="order-2 text-xs text-muted-foreground md:order-none md:text-sm"
						datetime={entry.at}
					>
						{formatWhen(entry.at)}
					</time>
					<span class="order-1 truncate font-medium md:order-none">
						{#if entry.titleId && entry.titleName}
							<a class="hover:underline" href={TITLE_PATH + entry.titleId}>{entry.titleName}</a>
						{:else}
							{entry.titleName}
						{/if}
					</span>
					<div class="order-3 col-span-2 min-w-0 md:order-none md:col-span-1">
						<span class={cn(alert && 'text-destructive')}>{entry.text}</span>
						{#if entry.detail}
							<span class="block truncate font-mono text-xs text-muted-foreground"
								>{entry.detail}</span
							>
						{/if}
					</div>
				</li>
			{/each}
		</ol>
		{#if feed.moreError}
			<p class="text-sm text-destructive" role="alert">{feed.moreError}</p>
		{/if}
		{#if feed.hasMore}
			<Button variant="ghost" size="sm" disabled={feed.loadingMore} onclick={() => feed.loadMore()}>
				{feed.loadingMore ? 'Loading…' : 'Show older'}
			</Button>
		{/if}
	{/if}
</section>
