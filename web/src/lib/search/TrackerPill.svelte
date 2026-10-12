<script lang="ts">
	import { MediaQuery } from 'svelte/reactivity';
	import Check from '@lucide/svelte/icons/check';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import Minus from '@lucide/svelte/icons/minus';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import X from '@lucide/svelte/icons/x';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import * as Popover from '$lib/components/ui/popover';
	import { IndexerStatus, type InteractiveSearch } from './interactive-search.svelte';

	let { search }: { search: InteractiveSearch } = $props();

	const HOVER_DELAY_MS = 80;
	const canHover = new MediaQuery('(hover: hover)');

	const SEGMENT_CLASS: Record<string, string> = {
		[IndexerStatus.PENDING]: 'animate-pulse bg-primary/30',
		[IndexerStatus.DONE]: 'bg-primary',
		[IndexerStatus.FAILED]: 'bg-destructive',
		[IndexerStatus.SKIPPED]: 'bg-muted-foreground/40'
	};

	const label = $derived.by(() => {
		if (search.completed && search.indexers.length === 0) return 'No trackers';
		if (search.completed) return 'All trackers answered';
		return `${search.answered} / ${search.indexers.length} trackers`;
	});
</script>

{#snippet pill(props: Record<string, unknown>)}
	<button
		{...props}
		type="button"
		class="flex items-center gap-3 rounded-full border px-3 py-1.5 hover:bg-muted/60"
		aria-label="Tracker status: {label}"
	>
		<span class="flex gap-0.5">
			{#each search.indexers as run (run.name)}
				<span class="h-3 w-1.5 rounded-sm transition-colors {SEGMENT_CLASS[run.status]}"></span>
			{/each}
		</span>
		<span class="text-xs tabular-nums">{label}</span>
		{#if search.failures > 0}
			<span class="flex items-center gap-1 text-xs text-destructive">
				<TriangleAlert class="size-3" />{search.failures}
			</span>
		{/if}
	</button>
{/snippet}

{#snippet trackers()}
	<ul>
		{#each search.indexers as run (run.name)}
			<li class="flex items-center gap-2 px-2 py-1.5 text-sm">
				{#if run.status === IndexerStatus.PENDING}
					<LoaderCircle class="size-4 animate-spin text-muted-foreground" />
				{:else if run.status === IndexerStatus.DONE}
					<Check class="size-4 text-primary" />
				{:else if run.status === IndexerStatus.SKIPPED}
					<Minus class="size-4 text-muted-foreground" />
				{:else}
					<X class="size-4 text-destructive" />
				{/if}
				<span class="truncate" title={run.message || undefined}>{run.name}</span>
			</li>
		{:else}
			<li class="px-2 py-1.5 text-sm text-muted-foreground">No indexers can search this title</li>
		{/each}
	</ul>
{/snippet}

{#if canHover.current}
	<HoverCard.Root openDelay={HOVER_DELAY_MS} closeDelay={HOVER_DELAY_MS}>
		<HoverCard.Trigger>
			{#snippet child({ props })}{@render pill(props)}{/snippet}
		</HoverCard.Trigger>
		<HoverCard.Content class="w-56 p-1" align="start">{@render trackers()}</HoverCard.Content>
	</HoverCard.Root>
{:else}
	<Popover.Root>
		<Popover.Trigger>
			{#snippet child({ props })}{@render pill(props)}{/snippet}
		</Popover.Trigger>
		<Popover.Content class="w-56 p-1" align="start">{@render trackers()}</Popover.Content>
	</Popover.Root>
{/if}
