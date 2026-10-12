<script lang="ts">
	import { onMount } from 'svelte';
	import ArrowDownToLine from '@lucide/svelte/icons/arrow-down-to-line';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import type { DownloadResponse } from '$lib/sdk';
	import { formatSize } from '$lib/search/format';
	import DownloadState from './DownloadState.svelte';
	import ProgressBar from './ProgressBar.svelte';
	import { DownloadsManager } from './downloads-manager.svelte';
	import { etaOrGoal, formatRatio, formatSpeed } from './downloads';

	const SKELETON_ROWS = 3;
	const COLUMNS = [
		'Title',
		'Release',
		'Indexer',
		'State',
		'Progress',
		'Speed',
		'ETA / goal',
		'Ratio',
		'Size',
		''
	];

	const manager = new DownloadsManager();
	const live = getLive();

	onMount(() => {
		void manager.load();
		return manager.watch(live);
	});
</script>

{#snippet removeButton(d: DownloadResponse)}
	<Button
		size="icon-sm"
		variant="ghost"
		title="Remove"
		aria-label={`Remove ${d.releaseName}`}
		disabled={manager.removing.has(d.id)}
		onclick={() => manager.askRemove(d)}
	>
		<Trash2 />
	</Button>
{/snippet}

{#if manager.status === LoadStatus.LOADING}
	<div class="flex flex-col gap-2" aria-busy="true" aria-label="Loading downloads">
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
{:else if manager.downloads.length === 0}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<ArrowDownToLine class="size-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">Nothing is downloading right now.</p>
	</section>
{:else}
	<div class="hidden rounded-xl border lg:block">
		<Table.Root>
			<Table.Header>
				<Table.Row>
					{#each COLUMNS as column, i (i)}
						<Table.Head class="whitespace-nowrap">{column}</Table.Head>
					{/each}
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each manager.downloads as d (d.id)}
					<Table.Row>
						<Table.Cell class="max-w-40 truncate font-medium" title={d.titleName}
							>{d.titleName}</Table.Cell
						>
						<Table.Cell class="max-w-72 truncate font-mono text-xs" title={d.releaseName}>
							{d.releaseName}
						</Table.Cell>
						<Table.Cell class="text-muted-foreground">{d.indexer}</Table.Cell>
						<Table.Cell><DownloadState state={d.state} /></Table.Cell>
						<Table.Cell class="w-40"><ProgressBar progress={d.progress} /></Table.Cell>
						<Table.Cell class="whitespace-nowrap tabular-nums">{formatSpeed(d.speedBps)}</Table.Cell
						>
						<Table.Cell class="whitespace-nowrap tabular-nums">{etaOrGoal(d)}</Table.Cell>
						<Table.Cell class="tabular-nums">{formatRatio(d.ratio)}</Table.Cell>
						<Table.Cell class="whitespace-nowrap tabular-nums">{formatSize(d.sizeBytes)}</Table.Cell
						>
						<Table.Cell class="text-right">{@render removeButton(d)}</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>

	<div class="flex flex-col gap-3 lg:hidden">
		{#each manager.downloads as d (d.id)}
			<article class="flex flex-col gap-2 rounded-xl border p-3">
				<div class="flex items-start justify-between gap-2">
					<div class="min-w-0">
						<h3 class="truncate text-sm font-medium">{d.titleName}</h3>
						<p class="font-mono text-xs break-all text-muted-foreground">{d.releaseName}</p>
					</div>
					{@render removeButton(d)}
				</div>
				<div class="flex items-center gap-2">
					<DownloadState state={d.state} />
					<span class="truncate text-xs text-muted-foreground">{d.indexer}</span>
				</div>
				<ProgressBar progress={d.progress} />
				<div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground tabular-nums">
					<span>{formatSpeed(d.speedBps)}</span>
					<span>{etaOrGoal(d)}</span>
					<span>Ratio {formatRatio(d.ratio)}</span>
					<span>{formatSize(d.sizeBytes)}</span>
				</div>
			</article>
		{/each}
	</div>
{/if}

<AlertDialog.Root
	open={manager.confirming !== undefined}
	onOpenChange={(open) => {
		if (!open) manager.confirming = undefined;
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Remove this download?</AlertDialog.Title>
			<AlertDialog.Description class="font-mono text-xs break-all">
				{manager.confirming?.releaseName}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<div class="flex items-center justify-between gap-4">
			<label for="delete-data" class="text-sm">Also delete the downloaded data</label>
			<Switch id="delete-data" bind:checked={manager.deleteData} />
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action
				class="bg-destructive text-white hover:bg-destructive/90"
				onclick={() => manager.confirmRemove()}
			>
				Remove
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
