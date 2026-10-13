<script lang="ts">
	import Info from '@lucide/svelte/icons/info';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import HoverPopover from '$lib/components/HoverPopover.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { formatWhen } from '$lib/activity/format';
	import type { MediaFileResponse } from '$lib/sdk';
	import { formatSize } from '$lib/search/format';
	import FileSubtitles from '$lib/subtitles/FileSubtitles.svelte';
	import {
		audioTracks,
		baseName,
		episodeLabel,
		FILE_SECTION_HEADING,
		formatDuration,
		videoSummary
	} from './library';
	import { CutoffState, type TitleFiles } from './title-files.svelte';

	let { file, files }: { file: MediaFileResponse; files: TitleFiles } = $props();

	const CUTOFF_BADGES: Record<CutoffState, { label: string; class: string } | undefined> = {
		[CutoffState.MET]: { label: 'Meets cutoff', class: 'border-primary/50 text-primary' },
		[CutoffState.UPGRADING]: {
			label: 'Below cutoff · upgrading',
			class: 'border-amber-500/50 text-amber-400'
		},
		[CutoffState.KEPT]: { label: 'Below cutoff · not upgrading', class: 'text-muted-foreground' },
		[CutoffState.UNKNOWN]: undefined
	};

	const cutoff = $derived(files.cutoff.get(file.id));
	const cutoffBadge = $derived(CUTOFF_BADGES[files.cutoffOf(file)]);
	const audio = $derived(audioTracks(file));
</script>

<article class="rounded-xl border bg-card">
	<div class="flex items-start gap-2 py-3 pr-2 pl-4">
		<div class="grid min-w-0 flex-1 gap-1.5">
			<span class="flex min-w-0 items-baseline gap-2">
				{#if file.episodes.length > 0}
					<span class="shrink-0 text-base font-semibold tabular-nums"
						>{episodeLabel(file.episodes)}</span
					>
				{/if}
				<span class="truncate font-mono text-sm text-muted-foreground" title={file.path}
					>{baseName(file.path)}</span
				>
			</span>
			<span class="flex flex-wrap items-center gap-1.5">
				<Badge variant="outline" class="border-primary/50 text-primary">{file.quality.name}</Badge>
				{#if cutoffBadge}
					<Badge variant="outline" class={cutoffBadge.class}>{cutoffBadge.label}</Badge>
				{/if}
				<span class="text-xs text-muted-foreground tabular-nums">
					{formatSize(file.size)}
					{#if file.mediaInfo.durationSeconds > 0}
						· {formatDuration(file.mediaInfo.durationSeconds)}
					{/if}
				</span>
			</span>
		</div>
		<HoverPopover class="w-96 max-w-[calc(100vw-2rem)]">
			{#snippet trigger(props)}
				<Button
					{...props}
					variant="ghost"
					size="icon-sm"
					class="shrink-0 text-muted-foreground"
					aria-label="File details"
				>
					<Info />
				</Button>
			{/snippet}
			<dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
				<dt class="text-muted-foreground">Path</dt>
				<dd class="font-mono break-all">{file.path}</dd>
				{#if file.releaseName}
					<dt class="text-muted-foreground">Release</dt>
					<dd class="font-mono break-all">{file.releaseName}</dd>
				{/if}
				{#if cutoff}
					<dt class="text-muted-foreground">CF score</dt>
					<dd class="tabular-nums">{cutoff.cfScore}</dd>
				{/if}
				<dt class="text-muted-foreground">Imported</dt>
				<dd>{formatWhen(file.importedAt)}</dd>
			</dl>
		</HoverPopover>
		<Button
			variant="ghost"
			size="icon-sm"
			class="shrink-0 text-muted-foreground hover:text-destructive"
			aria-label="Delete file"
			disabled={files.deleting.has(file.id)}
			onclick={() => files.askDelete(file)}
		>
			<Trash2 />
		</Button>
	</div>

	<div class="grid gap-4 border-t px-4 py-3">
		{#if file.renameProblem}
			<p class="flex items-start gap-1.5 text-xs text-amber-400" role="status">
				<TriangleAlert class="mt-px size-3.5 shrink-0" />
				Kept its old name after the episode order changed. {file.renameProblem}
			</p>
		{/if}
		<div class="grid gap-4 sm:grid-cols-2">
			<section class="grid content-start gap-1">
				<h3 class={FILE_SECTION_HEADING}>Video</h3>
				<p class="text-sm text-muted-foreground">{videoSummary(file)}</p>
			</section>
			<section class="grid content-start gap-1">
				<h3 class={FILE_SECTION_HEADING}>Audio</h3>
				<ul class="text-sm text-muted-foreground">
					{#each audio as track, i (i)}
						<li>{track}</li>
					{:else}
						<li>None</li>
					{/each}
				</ul>
			</section>
		</div>
		<FileSubtitles fileId={file.id} />
	</div>
</article>
