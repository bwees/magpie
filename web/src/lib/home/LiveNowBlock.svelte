<script lang="ts">
	import BlockState from '$lib/components/BlockState.svelte';
	import ThinProgress from '$lib/components/ThinProgress.svelte';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import type { LivePipeline } from '$lib/activity/live-pipeline.svelte';
	import { STAGE_LABELS, Stage, TranscodeState, transcodeChipText } from '$lib/activity/pipeline';
	import { episodeCode } from '$lib/catalog/episodes';
	import { ImageWidth, sizedImage, titlePath } from '$lib/catalog/images';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import type { TitleDirectory } from '$lib/catalog/title-directory.svelte';
	import { Route } from '$lib/navigation/nav';
	import type { PipelineRowResponse } from '$lib/sdk';
	import { cn } from '$lib/utils';

	let { pipeline, directory }: { pipeline: LivePipeline; directory: TitleDirectory } = $props();

	const SKELETON_CARDS = 2;
	/** Cards shown before the rest fold into a link to Activity. */
	const SHOWN_CARDS = 6;

	const shown = $derived(pipeline.rows.slice(0, SHOWN_CARDS));
	const hidden = $derived(pipeline.rows.length - shown.length);
	const UNKNOWN_TITLE = 'Unknown title';

	$effect(() => {
		void directory.ensure(pipeline.rows.map((row) => row.titleId));
	});

	function name(row: PipelineRowResponse): string {
		const title = directory.get(row.titleId)?.name ?? UNKNOWN_TITLE;
		return row.episodeId ? `${title} ${episodeCode(row.season, row.number)}` : title;
	}

	/** A file being transcoded is past the main stages, so its chip says what is running. */
	function stageText(row: PipelineRowResponse): string {
		if (row.stage === Stage.AVAILABLE && row.transcode?.state === TranscodeState.RUNNING) {
			return transcodeChipText(row.transcode, pipeline.progress.job(row));
		}
		return STAGE_LABELS[row.stage as Stage] ?? row.stage;
	}

	function detail(row: PipelineRowResponse, percent: number | undefined): string {
		return [row.detail, percent === undefined ? '' : `${percent}%`].filter(Boolean).join(' · ');
	}
</script>

<section aria-labelledby="live-now">
	<h2 id="live-now" class="mb-3 text-sm font-semibold">Live now</h2>
	<BlockState
		block={pipeline.block}
		label="what is running"
		empty="Nothing running"
		onRetry={() => pipeline.load()}
	>
		{#snippet skeleton()}
			<div class="flex flex-col gap-3">
				{#each { length: SKELETON_CARDS }, i (i)}
					<Skeleton class="h-20 w-full rounded-xl" />
				{/each}
			</div>
		{/snippet}
		<ul class="flex flex-col gap-3">
			{#each shown as row (`${row.titleId}:${row.episodeId ?? ''}`)}
				{@const percent = pipeline.progress.of(row)}
				{@const poster = directory.get(row.titleId)?.images.poster}
				<li>
					<a
						href={titlePath(row.titleId)}
						class="flex gap-3 rounded-xl border p-3 text-left transition-colors hover:bg-muted/50"
					>
						<PosterImage src={sizedImage(poster, ImageWidth.THUMB)} alt="" class="w-10 shrink-0" />
						<div class="min-w-0 flex-1">
							<div class="flex items-center justify-between gap-2">
								<span class="truncate text-sm font-medium">{name(row)}</span>
								<span class="shrink-0 text-xs text-muted-foreground">{stageText(row)}</span>
							</div>
							<div
								class={cn(
									'truncate text-xs text-muted-foreground',
									row.failed && 'text-destructive'
								)}
							>
								{detail(row, percent) || ' '}
							</div>
							<ThinProgress {percent} class="mt-2" />
						</div>
					</a>
				</li>
			{/each}
		</ul>
		{#if hidden > 0}
			<a href={Route.ACTIVITY} class="mt-3 block text-xs text-muted-foreground hover:underline">
				{hidden} more in Activity →
			</a>
		{/if}
	</BlockState>
</section>
