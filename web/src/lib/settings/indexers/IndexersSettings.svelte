<script lang="ts">
	import { onMount } from 'svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Rss from '@lucide/svelte/icons/rss';
	import { toast } from 'svelte-sonner';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import IndexerTableRow from './IndexerTableRow.svelte';
	import { formatSince, LIMITS } from './indexer-settings';
	import { IndexersManager, LoadState } from './indexers-manager.svelte';

	const SKELETON_ROWS = ['first', 'second', 'third'];
	const COLUMNS = [
		'Indexer',
		'Status',
		'Avg response',
		'Last OK',
		'Priority',
		'Min seeders',
		'After download',
		'Seeding goal, whichever first'
	];

	const manager = new IndexersManager((count) =>
		toast.success('Indexers synced', {
			description: `${count} ${count === 1 ? 'indexer' : 'indexers'} from Prowlarr`
		})
	);
	const now = new Date();

	onMount(() => {
		manager.load();
	});
</script>

<div class="grid gap-6">
	<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
		<div>
			<h2 class="text-xl font-semibold">Indexers</h2>
			<p class="text-sm text-muted-foreground">
				Indexers are managed in Prowlarr. Magpie queries each one directly.
			</p>
			{#if manager.lastSyncedAt}
				<p class="mt-1 text-xs text-muted-foreground">
					{manager.syncError ? 'Last attempt' : 'Last synced'}
					{formatSince(manager.lastSyncedAt, now)}
				</p>
			{/if}
		</div>
		<div class="flex shrink-0 gap-2">
			<Button
				variant="outline"
				size="sm"
				onclick={() => manager.syncNow()}
				disabled={manager.syncing || manager.loadState === LoadState.LOADING}
			>
				<RefreshCw class={manager.syncing ? 'animate-spin' : ''} />
				Sync now
			</Button>
			<Button
				variant="outline"
				size="sm"
				href={manager.prowlarrUrl || undefined}
				target="_blank"
				rel="noopener noreferrer"
				disabled={!manager.prowlarrUrl}
			>
				Open Prowlarr
				<ExternalLink />
			</Button>
		</div>
	</div>

	{#if manager.loadError}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Couldn't load indexers</Alert.Title>
			<Alert.Description>{manager.loadError}</Alert.Description>
		</Alert.Root>
	{:else if manager.syncError}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Prowlarr sync failed</Alert.Title>
			<Alert.Description>
				{manager.syncError}. The list below is from the last successful sync.
			</Alert.Description>
		</Alert.Root>
	{/if}

	{#if manager.isEmpty}
		<Card.Root class="flex flex-col items-center gap-2 px-6 py-12 text-center">
			<Rss class="size-8 text-muted-foreground" />
			<p class="font-medium">Prowlarr has no indexers</p>
			<p class="max-w-sm text-sm text-muted-foreground">
				Add torrent indexers in Prowlarr, then sync. Disabled and usenet indexers aren't listed.
			</p>
		</Card.Root>
	{:else if manager.showsTable}
		<Card.Root class="min-w-0 py-0">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						{#each COLUMNS as column (column)}
							<Table.Head class="whitespace-nowrap">{column}</Table.Head>
						{/each}
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#if manager.loadState === LoadState.LOADING}
						{#each SKELETON_ROWS as key (key)}
							<Table.Row>
								<Table.Cell colspan={COLUMNS.length}><Skeleton class="h-8 w-full" /></Table.Cell>
							</Table.Row>
						{/each}
					{:else}
						{#each manager.rows as row (row.indexer.id)}
							<IndexerTableRow {row} {manager} {now} />
						{/each}
					{/if}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{/if}

	{#if manager.options}
		<form
			class="grid gap-4"
			onsubmit={(event) => {
				event.preventDefault();
				manager.saveOptions();
			}}
		>
			<h3 class="text-sm font-semibold">All indexers</h3>
			<div class="grid gap-4 sm:grid-cols-3">
				<div class="grid gap-1.5">
					<Label for="search-timeout">Search timeout</Label>
					<div class="flex items-center gap-2">
						<Input
							id="search-timeout"
							type="number"
							class="w-24"
							min={LIMITS.SEARCH_TIMEOUT_SECONDS.MIN}
							max={LIMITS.SEARCH_TIMEOUT_SECONDS.MAX}
							bind:value={manager.options.searchTimeoutSeconds}
						/>
						<span class="text-sm text-muted-foreground">seconds</span>
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="result-cache">Result cache</Label>
					<div class="flex items-center gap-2">
						<Input
							id="result-cache"
							type="number"
							class="w-24"
							min={LIMITS.RESULT_CACHE_MINUTES.MIN}
							max={LIMITS.RESULT_CACHE_MINUTES.MAX}
							bind:value={manager.options.resultCacheMinutes}
						/>
						<span class="text-sm text-muted-foreground">minutes</span>
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="sync-interval">Sync from Prowlarr every</Label>
					<div class="flex items-center gap-2">
						<Input
							id="sync-interval"
							type="number"
							class="w-24"
							min={LIMITS.SYNC_INTERVAL_MINUTES.MIN}
							max={LIMITS.SYNC_INTERVAL_MINUTES.MAX}
							bind:value={manager.options.syncIntervalMinutes}
						/>
						<span class="text-sm text-muted-foreground">minutes</span>
					</div>
				</div>
			</div>
			<div class="flex items-center gap-3">
				<Button type="submit" size="sm" disabled={manager.savingOptions}>Save</Button>
				{#if manager.optionsError}
					<span class="text-sm text-destructive" role="alert">{manager.optionsError}</span>
				{/if}
			</div>
		</form>
	{/if}
</div>
