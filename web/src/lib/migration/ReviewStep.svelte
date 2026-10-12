<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import ClashQueue from './ClashQueue.svelte';
	import { QUEUE_LABELS, QUEUE_SKIP_LABELS, ReviewQueue } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';
	import NumberingQueue from './NumberingQueue.svelte';
	import SeriesTypeQueue from './SeriesTypeQueue.svelte';
	import SpecQueue from './SpecQueue.svelte';
	import TitleQueue from './TitleQueue.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const queues = $derived(wizard.state?.reviews?.queues ?? []);
	const firstOpen = $derived(queues.find((q) => q.pending > 0)?.queue ?? ReviewQueue.TITLES);
	let tab = $state<string>();
	const shown = $derived(tab ?? firstOpen);
	const current = $derived(queues.find((q) => q.queue === shown));
</script>

<div class="grid gap-4 [&>*]:min-w-0">
	<p class="text-sm text-muted-foreground">
		Settle each queue, or skip what's left of it. Commit waits until every queue is done.
	</p>
	<Tabs.Root value={shown} onValueChange={(value) => (tab = value)} class="min-w-0 gap-4">
		<div class="-mx-1 overflow-x-auto px-1">
			<Tabs.List>
				{#each queues as queue (queue.queue)}
					<Tabs.Trigger value={queue.queue}>
						{QUEUE_LABELS[queue.queue as ReviewQueue]}
						{#if queue.pending > 0}
							<Badge class="ml-1 h-5 min-w-5 px-1.5 tabular-nums" aria-label="{queue.pending} open"
								>{queue.pending}</Badge
							>
						{/if}
					</Tabs.Trigger>
				{/each}
			</Tabs.List>
		</div>
		<Tabs.Content value={ReviewQueue.TITLES}><TitleQueue {wizard} /></Tabs.Content>
		<Tabs.Content value={ReviewQueue.NUMBERING}><NumberingQueue {wizard} /></Tabs.Content>
		<Tabs.Content value={ReviewQueue.CLASHES}><ClashQueue {wizard} /></Tabs.Content>
		<Tabs.Content value={ReviewQueue.SPECS}><SpecQueue {wizard} /></Tabs.Content>
		<Tabs.Content value={ReviewQueue.SERIES_TYPES}><SeriesTypeQueue {wizard} /></Tabs.Content>
	</Tabs.Root>
	<div class="flex flex-wrap justify-end gap-2">
		{#if current && current.pending > 0}
			<Button
				variant="outline"
				disabled={wizard.busy}
				onclick={() => wizard.skipQueue(current.queue as ReviewQueue)}
			>
				{QUEUE_SKIP_LABELS[current.queue as ReviewQueue]}
			</Button>
		{/if}
		<Button disabled={wizard.pendingReviews > 0} onclick={() => wizard.next()}>Continue</Button>
	</div>
</div>
