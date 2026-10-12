<script lang="ts">
	import FolderSearch from '@lucide/svelte/icons/folder-search';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { formatWhen } from '$lib/activity/format';
	import { LoadStatus } from '$lib/load-status';
	import type { LibraryScanManager } from './library-scan.svelte';

	let { scan, inReview }: { scan: LibraryScanManager; inReview: number } = $props();

	const counts = $derived([
		{ label: 'Found', value: scan.scan?.found ?? 0 },
		{ label: 'In the library', value: scan.scan?.linked ?? 0 },
		{ label: 'In review', value: inReview },
		{ label: 'No video', value: scan.scan?.ignored ?? 0 }
	]);
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Scan existing library</Card.Title>
		<Card.Description>
			Finds folders in the root folders that Magpie doesn't know yet. A folder tagged
			<code class="font-mono text-xs">[tmdbid-N]</code> matches at once; others match on title and year,
			or wait below for you.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-4">
		{#if scan.status === LoadStatus.LOADING}
			<Skeleton class="h-24 w-full rounded-lg" />
		{:else if scan.status === LoadStatus.ERROR}
			<div class="flex flex-wrap items-center gap-2" role="alert">
				<span class="text-sm text-destructive">{scan.error}</span>
				<Button size="sm" variant="outline" onclick={() => scan.load()}>Retry</Button>
			</div>
		{:else}
			<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<p class="text-sm text-muted-foreground" role="status">
					{#if scan.running}
						<LoaderCircle class="mr-1 inline size-4 animate-spin align-text-bottom text-primary" />
						Scanning the root folders…
					{:else if scan.scan?.finishedAt}
						Last scan {formatWhen(scan.scan.finishedAt)} reported {scan.scan.reported}
						{scan.scan.reported === 1 ? 'folder' : 'folders'}.
					{:else}
						The library hasn't been scanned yet.
					{/if}
				</p>
				<Button
					variant="outline"
					disabled={scan.running || scan.starting}
					onclick={() => scan.start()}
				>
					<FolderSearch />
					{scan.running ? 'Scanning…' : 'Scan library'}
				</Button>
			</div>
			{#if scan.scan?.error}
				<p class="text-sm text-destructive" role="alert">
					The last scan stopped: {scan.scan.error}
				</p>
			{/if}
			<dl class="grid grid-cols-2 gap-2 sm:grid-cols-4" aria-label="Scanned folders">
				{#each counts as count (count.label)}
					<div class="rounded-lg border p-3">
						<dt class="text-xs text-muted-foreground">{count.label}</dt>
						<dd class="text-xl font-semibold tabular-nums">{count.value}</dd>
					</div>
				{/each}
			</dl>
			<div class="flex items-start justify-between gap-4 rounded-lg border p-3">
				<div class="grid gap-0.5">
					<label for="rename-matched" class="text-sm font-medium"
						>Rename matched folders on scan</label
					>
					<p class="text-xs text-muted-foreground">
						Moves matched videos and their subtitles to the naming below. Off, files stay where they
						are.
					</p>
				</div>
				<Switch
					id="rename-matched"
					checked={scan.scan?.renameMatched ?? false}
					onCheckedChange={(checked) => scan.setRenameMatched(checked)}
				/>
			</div>
		{/if}
	</Card.Content>
</Card.Root>
