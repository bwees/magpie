<script lang="ts">
	import Captions from '@lucide/svelte/icons/captions';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Search from '@lucide/svelte/icons/search';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { cn } from '$lib/utils';
	import CandidatePicker from './CandidatePicker.svelte';
	import { FileSubtitles } from './file-subtitles.svelte';
	import {
		CoverageStatus,
		coverageLabel,
		fileName,
		SOURCE_LABELS,
		SubtitleSource,
		syncLabel,
		trackLabel
	} from './subtitles';

	let { fileId }: { fileId: string } = $props();

	const live = getLive();
	const subs = $derived(new FileSubtitles(fileId));

	$effect(() => {
		void subs.load();
		return subs.watch(live);
	});

	const COVERAGE_TONES: Record<string, string> = {
		[CoverageStatus.SATISFIED]: 'border-primary/50 text-primary',
		[CoverageStatus.PENDING]: 'border-muted-foreground/40 text-muted-foreground',
		[CoverageStatus.MISSING]: 'border-amber-500/50 text-amber-500'
	};
</script>

<section class="grid gap-2" aria-label="Subtitles">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h3 class="text-xs text-muted-foreground">Subtitles</h3>
		<div class="flex gap-1.5">
			<Button
				size="sm"
				variant="outline"
				disabled={subs.searching || subs.status !== LoadStatus.READY}
				onclick={() => subs.searchNow()}
			>
				<RefreshCw class={cn(subs.searching && 'animate-spin')} />Search now
			</Button>
			<Button
				size="sm"
				variant="outline"
				disabled={subs.status !== LoadStatus.READY}
				onclick={() => subs.openPicker()}
			>
				<Search />Find…
			</Button>
		</div>
	</div>

	{#if subs.status === LoadStatus.LOADING}
		<Skeleton class="h-16 w-full" aria-label="Loading subtitles" />
	{:else if subs.status === LoadStatus.ERROR || !subs.data}
		<div class="flex flex-wrap items-center gap-3" role="alert">
			<p class="text-sm text-destructive">{subs.error}</p>
			<Button size="sm" variant="outline" onclick={() => subs.load()}>Retry</Button>
		</div>
	{:else}
		{#if subs.data.coverage.length > 0}
			<ul class="flex flex-wrap gap-1.5" aria-label="Wanted languages">
				{#each subs.data.coverage as c (`${c.language}-${c.forced}`)}
					<li>
						<button
							type="button"
							class="rounded-md"
							title="Find {trackLabel(c)} subtitles"
							onclick={() => subs.openPicker(c.language, c.forced)}
						>
							<Badge variant="outline" class={COVERAGE_TONES[c.status]}>
								{trackLabel(c)} · {coverageLabel(c)}
							</Badge>
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		{#if subs.data.subtitles.length === 0}
			<p class="flex items-center gap-2 text-sm text-muted-foreground">
				<Captions class="size-4" />No subtitles yet.
			</p>
		{:else}
			<ul class="grid gap-1.5" aria-label="Subtitle tracks">
				{#each subs.data.subtitles as sub (sub.id)}
					{@const sync = syncLabel(sub)}
					<li
						class="flex flex-col gap-1 rounded-lg border px-3 py-2 sm:flex-row sm:items-center sm:gap-3"
					>
						<div class="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1">
							<span class="text-sm font-medium">{trackLabel(sub)}</span>
							<Badge variant="secondary">{SOURCE_LABELS[sub.source] ?? sub.source}</Badge>
							{#if sub.score !== undefined}
								<span class="text-xs text-muted-foreground tabular-nums">{sub.score}%</span>
							{/if}
							{#if sync}
								<span class={cn('text-xs', sub.synced ? 'text-primary' : 'text-muted-foreground')}
									>{sync}</span
								>
							{/if}
						</div>
						{#if sub.path}
							<span
								class="min-w-0 font-mono text-xs break-all text-muted-foreground sm:max-w-[45%] sm:truncate"
								title={sub.path}>{fileName(sub.path)}</span
							>
						{/if}
						{#if sub.source !== SubtitleSource.EMBEDDED}
							<Button
								size="icon-sm"
								variant="ghost"
								class="self-end sm:self-auto"
								aria-label="Remove {trackLabel(sub)} subtitle"
								disabled={subs.removing === sub.id}
								onclick={() => subs.remove(sub.id)}
							>
								<Trash2 />
							</Button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{/if}
</section>

<CandidatePicker {subs} />
