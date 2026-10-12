<script lang="ts">
	import FileVideo from '@lucide/svelte/icons/file-video';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { formatWhen } from '$lib/activity/format';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { formatSize } from '$lib/search/format';
	import FileSubtitles from '$lib/subtitles/FileSubtitles.svelte';
	import { audioTracks, episodeLabel, formatDuration, videoSummary } from './library';
	import { CutoffState, TitleFiles } from './title-files.svelte';

	let { titleId }: { titleId: string } = $props();

	const SKELETON_CARDS = 2;
	const CUTOFF_BADGES: Record<CutoffState, { label: string; class: string } | undefined> = {
		[CutoffState.MET]: { label: 'Meets cutoff', class: 'border-primary/50 text-primary' },
		[CutoffState.UPGRADING]: {
			label: 'Below cutoff · upgrading',
			class: 'border-amber-500/50 text-amber-400'
		},
		[CutoffState.KEPT]: { label: 'Below cutoff · not upgrading', class: 'text-muted-foreground' },
		[CutoffState.UNKNOWN]: undefined
	};
	const live = getLive();
	const files = $derived(new TitleFiles(titleId));

	$effect(() => {
		void files.load();
		return files.watch(live);
	});
</script>

{#snippet trackList(label: string, tracks: string[])}
	<div class="grid gap-0.5">
		<dt class="text-xs text-muted-foreground">{label}</dt>
		{#if tracks.length === 0}
			<dd class="text-sm text-muted-foreground">None</dd>
		{:else}
			{#each tracks as track, i (i)}
				<dd class="text-sm">{track}</dd>
			{/each}
		{/if}
	</div>
{/snippet}

{#if files.status === LoadStatus.LOADING}
	<div class="grid gap-3" aria-busy="true" aria-label="Loading files">
		{#each { length: SKELETON_CARDS }, i (i)}
			<Skeleton class="h-40 w-full rounded-xl" />
		{/each}
	</div>
{:else if files.status === LoadStatus.ERROR}
	<section
		class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center"
		role="alert"
	>
		<p class="text-sm text-muted-foreground">{files.error}</p>
		<Button variant="outline" size="sm" onclick={() => files.load()}>Retry</Button>
	</section>
{:else if files.files.length === 0}
	<section class="flex flex-col items-center gap-3 rounded-xl border border-dashed p-8 text-center">
		<FileVideo class="size-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">No files yet.</p>
	</section>
{:else}
	<div class="grid gap-3">
		{#if files.overdue.length > 0}
			<div
				class="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200"
				role="status"
			>
				<TriangleAlert class="mt-0.5 size-4 shrink-0" />
				<p>
					Jellyfin hasn't picked up {files.overdue.length === 1
						? 'this file'
						: `${files.overdue.length} of these files`} a day after import. Check that Jellyfin's library
					includes the root folder at the same path Magpie uses.
				</p>
			</div>
		{/if}
		{#each files.files as file (file.id)}
			{@const cutoff = files.cutoff.get(file.id)}
			{@const cutoffBadge = CUTOFF_BADGES[files.cutoffOf(file)]}
			<Card.Root class="gap-3 py-4">
				<Card.Header class="gap-1 px-4">
					<Card.Action>
						<Button
							variant="ghost"
							size="icon"
							class="size-8 text-muted-foreground hover:text-destructive"
							aria-label="Delete file"
							disabled={files.deleting.has(file.id)}
							onclick={() => files.askDelete(file)}
						>
							<Trash2 class="size-4" />
						</Button>
					</Card.Action>
					<div class="flex flex-wrap items-center gap-2">
						{#if file.episodes.length > 0}
							<Badge variant="secondary">{episodeLabel(file.episodes)}</Badge>
						{/if}
						<Badge variant="outline" class="border-primary/50 text-primary"
							>{file.quality.name}</Badge
						>
						{#if cutoffBadge}
							<Badge variant="outline" class={cutoffBadge.class}>{cutoffBadge.label}</Badge>
						{/if}
						{#if cutoff}
							<span class="text-xs text-muted-foreground">CF {cutoff.cfScore}</span>
						{/if}
						{#if file.available}
							<Badge variant="secondary">In Jellyfin</Badge>
						{:else if file.jellyfinOverdue}
							<Badge variant="outline" class="border-amber-500/50 text-amber-400"
								>Not in Jellyfin</Badge
							>
						{:else}
							<Badge variant="outline" class="text-muted-foreground">Waiting for Jellyfin</Badge>
						{/if}
						<span class="text-xs text-muted-foreground">
							{formatSize(file.size)}
							{#if file.mediaInfo.durationSeconds > 0}
								· {formatDuration(file.mediaInfo.durationSeconds)}
							{/if}
							· imported {formatWhen(file.importedAt)}
						</span>
					</div>
					<Card.Title class="font-mono text-xs font-normal break-all">{file.path}</Card.Title>
					{#if file.renameProblem}
						<p class="flex items-start gap-1.5 text-xs text-amber-400" role="status">
							<TriangleAlert class="mt-px size-3.5 shrink-0" />
							Kept its old name after the episode order changed. {file.renameProblem}
						</p>
					{/if}
					{#if file.releaseName}
						<Card.Description class="font-mono text-xs break-all"
							>{file.releaseName}</Card.Description
						>
					{/if}
				</Card.Header>
				<Card.Content class="grid gap-4 px-4">
					<dl class="grid gap-3 sm:grid-cols-2">
						{@render trackList('Video', [videoSummary(file)])}
						{@render trackList('Audio', audioTracks(file))}
					</dl>
					<FileSubtitles fileId={file.id} />
				</Card.Content>
			</Card.Root>
		{/each}
	</div>
{/if}

<AlertDialog.Root
	open={files.confirmingDelete !== undefined}
	onOpenChange={(open) => {
		if (!open) files.cancelDelete();
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete this file?</AlertDialog.Title>
			<AlertDialog.Description>
				Magpie deletes the video from disk along with its subtitles. A monitored title is searched
				for again in the next missing sweep.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<p class="font-mono text-xs break-all">{files.confirmingDelete?.path}</p>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action
				class="bg-destructive text-white hover:bg-destructive/90"
				onclick={() => files.confirmDelete()}
			>
				Delete file
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
