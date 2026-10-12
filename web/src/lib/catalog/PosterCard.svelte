<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import ThinProgress from '$lib/components/ThinProgress.svelte';
	import { STAGE_LABELS, Stage } from '$lib/activity/pipeline';
	import type { PipelineRowResponse } from '$lib/sdk';
	import { ImageWidth, sizedImage, titlePath } from './images';
	import PosterImage from './PosterImage.svelte';
	import { MediaKind } from './title-page.svelte';

	let {
		titleId,
		name,
		year,
		kind,
		poster,
		stage,
		percent
	}: {
		titleId: string;
		name: string;
		year?: number;
		kind?: string;
		poster: string | null | undefined;
		/** The title's summary row; its stage shows unless the title is Available. */
		stage?: PipelineRowResponse;
		/** Live progress, overlaid on the poster. */
		percent?: number;
	} = $props();

	const KIND_LABELS: Record<string, string> = {
		[MediaKind.MOVIE]: 'Movie',
		[MediaKind.SERIES]: 'Series'
	};

	const meta = $derived(
		[year || undefined, kind ? KIND_LABELS[kind] : undefined].filter(Boolean).join(' · ')
	);
</script>

<a href={titlePath(titleId)} class="group flex min-w-0 flex-col text-left">
	<div class="relative">
		<PosterImage
			src={sizedImage(poster, ImageWidth.CARD)}
			alt={name}
			class="transition group-hover:scale-[1.02] group-hover:ring-2 group-hover:ring-primary/50"
		/>
		{#if stage && stage.stage !== Stage.AVAILABLE}
			<Badge
				variant={stage.failed ? 'destructive' : 'default'}
				class="absolute {stage.failed
					? 'bg-destructive text-white dark:bg-destructive'
					: ''} top-1.5 left-1.5 max-w-[calc(100%-0.75rem)] truncate text-[10px] sm:top-2 sm:left-2 sm:text-xs"
			>
				{STAGE_LABELS[stage.stage as Stage] ?? stage.stage}
			</Badge>
		{/if}
		{#if percent !== undefined}
			<ThinProgress {percent} class="absolute right-2 bottom-2 left-2 bg-black/50" />
		{/if}
	</div>
	<span class="mt-2 truncate text-sm font-medium" title={name}>{name}</span>
	{#if meta}
		<span class="text-xs text-muted-foreground">{meta}</span>
	{/if}
</a>
