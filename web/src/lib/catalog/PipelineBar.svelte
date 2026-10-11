<script lang="ts">
	import { cn } from '$lib/utils';
	import {
		PIPELINE_STAGES,
		SegmentState,
		segmentStates,
		stepLabel,
		type PipelineStage
	} from './pipeline';

	let { stage }: { stage?: PipelineStage } = $props();

	const SEGMENT_FILL: Record<SegmentState, string> = {
		[SegmentState.DONE]: 'w-full',
		[SegmentState.CURRENT]: 'w-1/2',
		[SegmentState.PENDING]: 'w-0'
	};

	const NO_DATA = '—';

	const states = $derived(segmentStates(stage));
</script>

<div class="flex flex-col gap-3 md:flex-row md:items-start md:gap-6">
	<div class="min-w-0 flex-1">
		<p class="mb-2 text-xs font-medium text-white/80 md:hidden">{stepLabel(stage)}</p>
		<ol class="grid grid-cols-5 gap-1.5" aria-label="Pipeline stage: {stepLabel(stage)}">
			{#each PIPELINE_STAGES as { stage: segment, label }, index (segment)}
				<li class="flex min-w-0 flex-col gap-1.5">
					<span class="h-1.5 overflow-hidden rounded-full bg-white/20">
						<span class={cn('block h-full rounded-full bg-primary', SEGMENT_FILL[states[index]])}
						></span>
					</span>
					<span
						class={cn(
							'hidden truncate text-xs md:block',
							states[index] === SegmentState.PENDING ? 'text-white/60' : 'text-white'
						)}
					>
						{label}
					</span>
				</li>
			{/each}
		</ol>
	</div>
	<div class="flex flex-wrap gap-2 text-xs">
		<span class="rounded-full bg-white/10 px-2.5 py-0.5 text-white/70">Subs {NO_DATA}</span>
		<span class="rounded-full bg-white/10 px-2.5 py-0.5 text-white/70">Transcode {NO_DATA}</span>
	</div>
</div>
