<script lang="ts">
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { cn } from '$lib/utils';
	import { LoadStatus, SDK_SOURCE, type ActivitySource } from './activity-source';
	import { formatWhen, isAlert } from './format';
	import { TitleTimeline } from './title-timeline.svelte';

	const SKELETON_CARDS = 4;

	let {
		titleId,
		source = SDK_SOURCE,
		class: className
	}: { titleId: string; source?: ActivitySource; class?: string } = $props();

	const live = getLive();
	const timeline = $derived(new TitleTimeline(titleId, source));

	$effect(() => {
		const current = timeline;
		return untrack(() => {
			void current.load();
			return current.connect(live);
		});
	});
</script>

<section class={cn('flex flex-col gap-2', className)} aria-label="Timeline">
	{#if timeline.status === LoadStatus.LOADING}
		{#each { length: SKELETON_CARDS }, index (index)}
			<Skeleton class="h-16 w-full rounded-lg" />
		{/each}
	{:else if timeline.status === LoadStatus.ERROR}
		<div class="flex items-center gap-3 text-sm text-destructive" role="alert">
			<span>Couldn't load the timeline. {timeline.error}</span>
			<Button size="sm" variant="outline" onclick={() => timeline.retry()}>Retry</Button>
		</div>
	{:else if timeline.empty}
		<p class="text-sm text-muted-foreground">Nothing has happened yet.</p>
	{:else}
		<ol class="flex flex-col gap-2">
			{#each timeline.shown as entry (entry.eventId)}
				{@const alert = isAlert(entry.severity)}
				<li
					class={cn(
						'flex flex-col gap-1 rounded-lg border bg-card px-3 py-2',
						alert && 'border-destructive/60 bg-destructive/10'
					)}
				>
					<div class="flex items-baseline justify-between gap-3">
						<span class={cn('text-sm font-medium', alert && 'text-destructive')}>{entry.text}</span>
						<time class="shrink-0 text-xs text-muted-foreground" datetime={entry.at}>
							{formatWhen(entry.at)}
						</time>
					</div>
					{#if entry.detail}
						<p class="font-mono text-xs break-all whitespace-pre-line text-muted-foreground">
							{entry.detail}
						</p>
					{/if}
				</li>
			{/each}
		</ol>
		{#if timeline.moreError}
			<p class="text-sm text-destructive" role="alert">{timeline.moreError}</p>
		{/if}
		{#if timeline.canExpand}
			<Button variant="ghost" size="sm" onclick={() => timeline.expand()}>Show more</Button>
		{:else if timeline.canLoadMore}
			<Button
				variant="ghost"
				size="sm"
				disabled={timeline.loadingMore}
				onclick={() => timeline.loadMore()}
			>
				{timeline.loadingMore ? 'Loading…' : 'Show older'}
			</Button>
		{/if}
	{/if}
</section>
