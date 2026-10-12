<script lang="ts">
	import PackageCheck from '@lucide/svelte/icons/package-check';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import { formatWhen } from '$lib/activity/format';
	import { LoadStatus } from '$lib/load-status';
	import type { ImportResponse } from '$lib/sdk';
	import { IMPORT_STATUS_LABELS, ImportStatus } from './library';
	import type { ImportsManager } from './imports-manager.svelte';
	import MapFilesDialog from './MapFilesDialog.svelte';

	let { manager }: { manager: ImportsManager } = $props();

	const SKELETON_ROWS = 3;
	const COLUMNS = ['Title', 'Release', 'Reason', 'Updated', ''];
</script>

{#snippet status(imp: ImportResponse)}
	<Badge variant={imp.status === ImportStatus.FAILED ? 'destructive' : 'secondary'}>
		{IMPORT_STATUS_LABELS[imp.status] ?? imp.status}
	</Badge>
{/snippet}

{#snippet actions(imp: ImportResponse)}
	{@const busy = manager.busy.has(imp.download)}
	<div class="flex flex-wrap justify-end gap-2">
		<Button size="sm" disabled={busy} onclick={() => manager.openMapping(imp)}>Map files</Button>
		<Button size="sm" variant="outline" disabled={busy} onclick={() => manager.retry(imp)}>
			Retry
		</Button>
		<Button
			size="sm"
			variant="outline"
			disabled={busy}
			title="The release is already blocklisted; this drops it from the queue"
			onclick={() => manager.remove(imp)}
		>
			Remove
		</Button>
	</div>
{/snippet}

{#if manager.status === LoadStatus.LOADING}
	<div class="flex flex-col gap-2" aria-busy="true" aria-label="Loading imports">
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
{:else if manager.imports.length === 0}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<PackageCheck class="size-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">Every download imported cleanly.</p>
	</section>
{:else}
	<p class="mb-3 text-sm text-muted-foreground">
		Magpie blocklisted these releases and looked for others. Map the files to import one anyway.
	</p>
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
				{#each manager.imports as imp (imp.download)}
					<Table.Row>
						<Table.Cell class="max-w-40">
							<div class="truncate font-medium">{imp.titleName}</div>
							{@render status(imp)}
						</Table.Cell>
						<Table.Cell class="max-w-72 truncate font-mono text-xs" title={imp.releaseName}>
							{imp.releaseName}
						</Table.Cell>
						<Table.Cell class="max-w-72 text-sm whitespace-normal">{imp.reason}</Table.Cell>
						<Table.Cell class="whitespace-nowrap text-muted-foreground">
							{formatWhen(imp.updatedAt)}
						</Table.Cell>
						<Table.Cell>{@render actions(imp)}</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>

	<div class="flex flex-col gap-3 md:hidden">
		{#each manager.imports as imp (imp.download)}
			<article class="flex flex-col gap-2 rounded-xl border p-3">
				<div class="flex items-center justify-between gap-2">
					<span class="truncate font-medium">{imp.titleName}</span>
					{@render status(imp)}
				</div>
				<p class="font-mono text-xs break-all">{imp.releaseName}</p>
				<p class="text-sm">{imp.reason}</p>
				<span class="text-xs text-muted-foreground">{formatWhen(imp.updatedAt)}</span>
				{@render actions(imp)}
			</article>
		{/each}
	</div>
{/if}

<MapFilesDialog {manager} />
