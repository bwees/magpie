<script lang="ts">
	import { onMount } from 'svelte';
	import ShieldCheck from '@lucide/svelte/icons/shield-check';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import { formatWhen } from '$lib/activity/format';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import type { BlocklistEntryResponse } from '$lib/sdk';
	import { BlocklistManager } from './blocklist-manager.svelte';
	import { REASON_LABELS } from './downloads';

	const SKELETON_ROWS = 3;
	const COLUMNS = ['Release', 'Title', 'Reason', 'Date', ''];

	const manager = new BlocklistManager();
	const live = getLive();

	onMount(() => {
		void manager.load();
		return manager.watch(live);
	});
</script>

{#snippet reason(entry: BlocklistEntryResponse)}
	<span class="flex flex-wrap items-center gap-1">
		<Badge variant="destructive">{REASON_LABELS[entry.reason] ?? entry.reason}</Badge>
		{#if entry.expiresAt}
			<span class="text-xs text-muted-foreground">
				{manager.isExpired(entry) ? 'expired' : `until ${formatWhen(entry.expiresAt)}`}
			</span>
		{/if}
	</span>
{/snippet}

{#snippet removeButton(entry: BlocklistEntryResponse)}
	<Button
		size="sm"
		variant="outline"
		disabled={manager.removing.has(entry.id)}
		onclick={() => manager.remove(entry)}
	>
		Remove
	</Button>
{/snippet}

{#if manager.status === LoadStatus.LOADING}
	<div class="flex flex-col gap-2" aria-busy="true" aria-label="Loading the blocklist">
		{#each { length: SKELETON_ROWS }, i (i)}
			<Skeleton class="h-12 w-full rounded-lg" />
		{/each}
	</div>
{:else if manager.status === LoadStatus.ERROR}
	<section
		class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
		role="alert"
	>
		<p class="text-sm text-muted-foreground">{manager.error}</p>
		<Button variant="outline" size="sm" onclick={() => manager.load()}>Retry</Button>
	</section>
{:else if manager.entries.length === 0}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<ShieldCheck class="size-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">No releases are blocklisted.</p>
	</section>
{:else}
	<div class="hidden rounded-xl border md:block">
		<Table.Root>
			<Table.Header>
				<Table.Row>
					{#each COLUMNS as column, i (i)}
						<Table.Head class="whitespace-nowrap">{column}</Table.Head>
					{/each}
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each manager.entries as entry (entry.id)}
					<Table.Row>
						<Table.Cell class="max-w-96 truncate font-mono text-xs" title={entry.releaseName}>
							{entry.releaseName}
						</Table.Cell>
						<Table.Cell class="max-w-40 truncate">{entry.titleName}</Table.Cell>
						<Table.Cell>{@render reason(entry)}</Table.Cell>
						<Table.Cell class="whitespace-nowrap text-muted-foreground"
							>{formatWhen(entry.at)}</Table.Cell
						>
						<Table.Cell class="text-right">{@render removeButton(entry)}</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>

	<div class="flex flex-col gap-3 md:hidden">
		{#each manager.entries as entry (entry.id)}
			<article class="flex flex-col gap-2 rounded-xl border p-3">
				<p class="font-mono text-xs break-all">{entry.releaseName}</p>
				<div class="flex items-center justify-between gap-2">
					<div class="flex min-w-0 flex-col gap-1">
						<span class="truncate text-sm">{entry.titleName}</span>
						{@render reason(entry)}
						<span class="text-xs text-muted-foreground">{formatWhen(entry.at)}</span>
					</div>
					{@render removeButton(entry)}
				</div>
			</article>
		{/each}
	</div>
{/if}
