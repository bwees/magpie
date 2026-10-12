<script lang="ts">
	import { onMount } from 'svelte';
	import FolderPlus from '@lucide/svelte/icons/folder-plus';
	import Trash from '@lucide/svelte/icons/trash-2';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import {
		KIND_LABELS,
		LINK_LABELS,
		LinkMode,
		MediaKind,
		NAMING_EXAMPLES
	} from '$lib/library/library';
	import { LoadStatus } from '$lib/load-status';
	import type { RootFolderResponse } from '$lib/sdk';
	import { DownloadOptionsManager } from '../downloads/download-options-manager.svelte';
	import SeedingModeSelect from '../downloads/SeedingModeSelect.svelte';
	import { RootFoldersManager } from './root-folders-manager.svelte';

	const folders = new RootFoldersManager();
	const seeding = new DownloadOptionsManager();
	const SKELETON_ROWS = 2;

	onMount(() => {
		void folders.load();
		void seeding.load();
	});

	const LINK_CLASSES: Record<string, string> = {
		[LinkMode.HARDLINK]: 'border-primary/50 text-primary',
		[LinkMode.COPY]: 'border-amber-500/50 text-amber-400',
		[LinkMode.UNKNOWN]: 'text-muted-foreground'
	};

	function linkHint(folder: RootFolderResponse): string {
		switch (folder.link) {
			case LinkMode.HARDLINK:
				return 'Same filesystem as the downloads: seeding imports take no extra space.';
			case LinkMode.COPY:
				return 'Different filesystem from the downloads: seeding imports are copied.';
		}
		return 'Unknown until Magpie can see qBittorrent’s download folder.';
	}
</script>

<div class="grid gap-6">
	<Card.Root>
		<Card.Header>
			<Card.Title>Root folders</Card.Title>
			<Card.Description>
				Imports go to the first folder of their kind. Each folder must exist, be writable, and be
				visible to Jellyfin.
			</Card.Description>
		</Card.Header>
		<Card.Content class="grid gap-4">
			{#if folders.status === LoadStatus.LOADING}
				<div class="grid gap-2" aria-busy="true" aria-label="Loading root folders">
					{#each { length: SKELETON_ROWS }, i (i)}
						<Skeleton class="h-14 w-full rounded-lg" />
					{/each}
				</div>
			{:else if folders.status === LoadStatus.ERROR}
				<div class="flex flex-wrap items-center gap-2" role="alert">
					<span class="text-sm text-destructive">{folders.error}</span>
					<Button size="sm" variant="outline" onclick={() => folders.load()}>Retry</Button>
				</div>
			{:else}
				{#if folders.check?.downloadPath}
					<p class="text-sm text-muted-foreground">
						qBittorrent downloads to <code class="font-mono text-foreground"
							>{folders.check.downloadPath}</code
						>.
					</p>
				{/if}
				{#if folders.check?.problem}
					<p
						class="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200"
					>
						{folders.check.problem}
					</p>
				{/if}
				{#if folders.folders.length === 0}
					<p class="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">
						No root folders yet. Add one for movies and one for TV.
					</p>
				{:else}
					<ul class="grid gap-2" aria-label="Root folders">
						{#each folders.folders as folder (folder.id)}
							<li class="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center">
								<div class="grid min-w-0 flex-1 gap-1">
									<div class="flex flex-wrap items-center gap-2">
										<Badge variant="secondary">{KIND_LABELS[folder.kind] ?? folder.kind}</Badge>
										<code class="font-mono text-sm break-all">{folder.path}</code>
									</div>
									<p class="text-xs text-muted-foreground">{linkHint(folder)}</p>
								</div>
								<div class="flex items-center gap-2">
									<Badge variant="outline" class={LINK_CLASSES[folder.link]}>
										{LINK_LABELS[folder.link] ?? folder.link}
									</Badge>
									<Button
										size="icon"
										variant="ghost"
										aria-label="Remove {folder.path}"
										disabled={folders.removing.has(folder.id)}
										onclick={() => folders.remove(folder)}
									>
										<Trash />
									</Button>
								</div>
							</li>
						{/each}
					</ul>
				{/if}
				<form
					class="grid gap-2"
					onsubmit={(event) => {
						event.preventDefault();
						folders.add();
					}}
				>
					<div class="flex flex-col gap-2 sm:flex-row">
						<Select.Root
							type="single"
							value={folders.newKind}
							onValueChange={(value) => (folders.newKind = value as MediaKind)}
						>
							<Select.Trigger class="w-full sm:w-32" aria-label="Kind of titles">
								{KIND_LABELS[folders.newKind]}
							</Select.Trigger>
							<Select.Content>
								{#each Object.values(MediaKind) as kind (kind)}
									<Select.Item value={kind} label={KIND_LABELS[kind]} />
								{/each}
							</Select.Content>
						</Select.Root>
						<Input
							class="font-mono sm:flex-1"
							placeholder="/media/movies"
							aria-label="Root folder path"
							aria-invalid={folders.addError ? true : undefined}
							bind:value={folders.newPath}
						/>
						<Button type="submit" variant="outline" disabled={folders.adding}>
							<FolderPlus />Add
						</Button>
					</div>
					{#if folders.addError}
						<span class="text-sm text-destructive" role="alert">{folders.addError}</span>
					{/if}
				</form>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title><label for="library-seeding-mode">After download</label></Card.Title>
			<Card.Description>
				The global default; an indexer can override it in Settings → Indexers.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if seeding.status === LoadStatus.LOADING}
				<Skeleton class="h-9 w-56" />
			{:else if seeding.status === LoadStatus.ERROR}
				<div class="flex flex-wrap items-center gap-2" role="alert">
					<span class="text-sm text-destructive">{seeding.error}</span>
					<Button size="sm" variant="outline" onclick={() => seeding.load()}>Retry</Button>
				</div>
			{:else}
				<SeedingModeSelect manager={seeding} id="library-seeding-mode" />
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Naming</Card.Title>
			<Card.Description>Files follow Jellyfin's naming. It can't be changed.</Card.Description>
		</Card.Header>
		<Card.Content>
			<ul class="grid gap-1">
				{#each NAMING_EXAMPLES as example (example)}
					<li><code class="font-mono text-xs break-all">{example}</code></li>
				{/each}
			</ul>
		</Card.Content>
	</Card.Root>
</div>
