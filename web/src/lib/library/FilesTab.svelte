<script lang="ts">
	import FileVideo from '@lucide/svelte/icons/file-video';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { formatWhen } from '$lib/activity/format';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { formatSize } from '$lib/search/format';
	import {
		audioTracks,
		episodeLabel,
		formatDuration,
		subtitleTracks,
		videoSummary
	} from './library';
	import { TitleFiles } from './title-files.svelte';

	let { titleId }: { titleId: string } = $props();

	const SKELETON_CARDS = 2;
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
			<Card.Root class="gap-3 py-4">
				<Card.Header class="gap-1 px-4">
					<div class="flex flex-wrap items-center gap-2">
						{#if file.episodes.length > 0}
							<Badge variant="secondary">{episodeLabel(file.episodes)}</Badge>
						{/if}
						<Badge variant="outline" class="border-primary/50 text-primary"
							>{file.quality.name}</Badge
						>
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
					{#if file.releaseName}
						<Card.Description class="font-mono text-xs break-all"
							>{file.releaseName}</Card.Description
						>
					{/if}
				</Card.Header>
				<Card.Content class="px-4">
					<dl class="grid gap-3 sm:grid-cols-3">
						{@render trackList('Video', [videoSummary(file)])}
						{@render trackList('Audio', audioTracks(file))}
						{@render trackList('Subtitles', subtitleTracks(file))}
					</dl>
				</Card.Content>
			</Card.Root>
		{/each}
	</div>
{/if}
