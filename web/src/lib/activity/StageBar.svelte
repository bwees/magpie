<script lang="ts">
	import { untrack } from 'svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import { getLive } from '$lib/live/live-client.svelte';
	import { cn } from '$lib/utils';
	import { LoadStatus, SDK_SOURCE, type ActivitySource } from './activity-source';
	import {
		STAGES,
		STAGE_LABELS,
		SegmentState,
		Stage,
		SubsState,
		TranscodeState,
		phoneStageLabel,
		stageProgress,
		subsChipText,
		transcodeChipText
	} from './pipeline';
	import { TitlePipeline } from './title-pipeline.svelte';

	let {
		titleId,
		source = SDK_SOURCE,
		class: className
	}: { titleId: string; source?: ActivitySource; class?: string } = $props();

	const live = getLive();
	const pipeline = $derived(new TitlePipeline(titleId, source));

	$effect(() => {
		const current = pipeline;
		return untrack(() => {
			void current.load();
			return current.connect(live);
		});
	});

	const summary = $derived(pipeline.summary);
	const stage = $derived((summary?.row.stage as Stage | undefined) ?? Stage.REQUESTED);
	const failed = $derived(summary?.row.failed ?? false);
	const segments = $derived(summary ? stageProgress(stage) : STAGES.map(() => SegmentState.TODO));
	const detail = $derived(
		[
			summary?.row.detail,
			pipeline.downloadPercent === undefined ? '' : `${pipeline.downloadPercent}%`
		]
			.filter(Boolean)
			.join(' · ')
	);
</script>

<section
	class={cn('flex flex-col gap-3 md:flex-row md:items-start md:gap-6', className)}
	aria-label="Pipeline"
>
	{#if pipeline.status === LoadStatus.LOADING}
		<div class="flex flex-1 flex-col gap-2" data-testid="stage-bar-skeleton">
			<Skeleton class="h-1.5 w-full" />
			<Skeleton class="h-3 w-48" />
		</div>
	{:else if pipeline.status === LoadStatus.ERROR}
		<div class="flex flex-1 items-center gap-3 text-sm text-destructive" role="alert">
			<span>Couldn't load the pipeline. {pipeline.error}</span>
			<Button size="sm" variant="outline" onclick={() => pipeline.retry()}>Retry</Button>
		</div>
	{:else}
		<div class="flex min-w-0 flex-1 flex-col gap-1.5">
			{#if summary}
				<p class="text-sm font-medium md:hidden">{phoneStageLabel(stage)}</p>
			{/if}
			<ol class="grid grid-cols-5 gap-1.5">
				{#each STAGES as label, index (label)}
					{@const segment = segments[index]}
					<li
						class="flex flex-col gap-1.5"
						aria-current={segment === SegmentState.CURRENT ? 'step' : undefined}
					>
						<div class="h-1.5 overflow-hidden rounded-full bg-muted">
							<div
								class={cn(
									'h-full rounded-full',
									segment === SegmentState.DONE && 'w-full bg-primary',
									segment === SegmentState.CURRENT && 'w-1/2',
									segment === SegmentState.CURRENT && (failed ? 'bg-destructive' : 'bg-primary'),
									segment === SegmentState.TODO && 'w-0'
								)}
							></div>
						</div>
						<span
							class={cn(
								'hidden text-xs text-muted-foreground md:block',
								segment === SegmentState.CURRENT && 'font-medium text-foreground',
								segment === SegmentState.CURRENT && failed && 'text-destructive'
							)}
						>
							{STAGE_LABELS[label]}
						</span>
					</li>
				{/each}
			</ol>
			{#if pipeline.empty}
				<p class="text-sm text-muted-foreground">Nothing has happened yet.</p>
			{:else if detail}
				<p class={cn('text-sm text-muted-foreground', failed && 'text-destructive')}>{detail}</p>
			{/if}
		</div>

		{#if summary?.subs || summary?.transcode}
			<div class="flex flex-wrap gap-2 md:max-w-64 md:justify-end">
				{#if summary.subs}
					<Badge variant={summary.subs.state === SubsState.SETTLED ? 'outline' : 'secondary'}>
						{subsChipText(summary.subs)}
					</Badge>
				{/if}
				{#if summary.transcode}
					{@const chip = summary.transcode}
					{#if chip.state === TranscodeState.SKIPPED && chip.reason}
						<Tooltip.Provider>
							<Tooltip.Root>
								<Tooltip.Trigger>
									{#snippet child({ props })}
										<Badge {...props} variant="outline">{transcodeChipText(chip)}</Badge>
									{/snippet}
								</Tooltip.Trigger>
								<Tooltip.Content>{chip.reason}</Tooltip.Content>
							</Tooltip.Root>
						</Tooltip.Provider>
					{:else}
						<Badge
							variant={chip.state === TranscodeState.FAILED
								? 'destructive'
								: chip.state === TranscodeState.DONE
									? 'outline'
									: 'secondary'}
						>
							{transcodeChipText(chip, pipeline.transcodePercent)}
						</Badge>
					{/if}
				{/if}
			</div>
		{/if}
	{/if}
</section>
