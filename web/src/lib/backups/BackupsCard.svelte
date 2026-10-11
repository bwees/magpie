<script lang="ts">
	import { onMount } from 'svelte';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { BACKUP_KIND_LABELS, BackupKind, BackupsState, formatSize } from './backups-state.svelte';

	const SKELETON_ROWS = 3;
	const dateFormat = new Intl.DateTimeFormat(undefined, {
		dateStyle: 'medium',
		timeStyle: 'short'
	});

	const backups = new BackupsState();

	onMount(() => {
		backups.load();
	});
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Database backups</Card.Title>
		<Card.Description>A daily snapshot, plus one before every upgrade and restore.</Card.Description
		>
		<Card.Action>
			<Button
				size="sm"
				variant="outline"
				disabled={backups.creating || backups.status !== LoadStatus.READY}
				onclick={() => backups.create()}
			>
				{backups.creating ? 'Backing up…' : 'Back up now'}
			</Button>
		</Card.Action>
	</Card.Header>
	<Card.Content>
		{#if backups.status === LoadStatus.LOADING}
			<div class="grid gap-2" aria-busy="true">
				{#each { length: SKELETON_ROWS }, row (row)}
					<Skeleton class="h-10 w-full" />
				{/each}
			</div>
		{:else if backups.status === LoadStatus.ERROR}
			<div class="flex flex-wrap items-center gap-3">
				<p class="text-sm text-destructive">{backups.error}</p>
				<Button size="sm" variant="outline" onclick={() => backups.load()}>Retry</Button>
			</div>
		{:else if backups.backups.length === 0}
			<p class="text-sm text-muted-foreground">No backups yet.</p>
		{:else}
			<ul class="divide-y rounded-lg border text-sm">
				{#each backups.backups as backup (backup.name)}
					<li class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
						<span class="min-w-0 flex-1 basis-full sm:basis-auto">
							<span class="block truncate">{dateFormat.format(new Date(backup.createdAt))}</span>
							<span class="block truncate font-mono text-xs text-muted-foreground">
								{backup.name} · {formatSize(backup.size)}
							</span>
						</span>
						<Badge variant="outline">{BACKUP_KIND_LABELS[backup.kind as BackupKind]}</Badge>
						<Button
							size="sm"
							variant="ghost"
							class="ml-auto"
							disabled={backups.restoring !== undefined}
							onclick={() => backups.askToRestore(backup.name)}
						>
							Restore
						</Button>
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
</Card.Root>

<AlertDialog.Root
	open={backups.confirming !== undefined}
	onOpenChange={(open) => {
		if (!open) backups.cancelRestore();
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Restore this backup?</AlertDialog.Title>
			<AlertDialog.Description>
				Magpie restarts with {backups.confirming}. Changes since then are lost; the current database
				is kept as a backup first.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action variant="destructive" onclick={() => backups.restoreConfirmed()}>
				Restore
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
