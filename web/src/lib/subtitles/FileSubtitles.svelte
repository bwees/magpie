<script lang="ts">
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Search from '@lucide/svelte/icons/search';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import HoverPopover from '$lib/components/HoverPopover.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import { FILE_SECTION_HEADING } from '$lib/library/library';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { cn } from '$lib/utils';
	import CandidatePicker from './CandidatePicker.svelte';
	import { FileSubtitles } from './file-subtitles.svelte';
	import {
		CoverageStatus,
		coverageLabel,
		embeddedSummary,
		fileName,
		languageName,
		SOURCE_LABELS,
		SubtitleSource,
		syncLabel,
		trackLabel,
		wantedLanguages
	} from './subtitles';

	let { fileId }: { fileId: string } = $props();

	const SEARCH_LABEL = 'Search subtitles now';
	const FIND_LABEL = 'Pick a subtitle';

	const live = getLive();
	const subs = $derived(new FileSubtitles(fileId));

	$effect(() => {
		void subs.load();
		return subs.watch(live);
	});

	const UNMET_TONES: Record<string, string> = {
		[CoverageStatus.PENDING]: 'text-muted-foreground',
		[CoverageStatus.MISSING]: 'text-amber-500'
	};
</script>

<section class="grid gap-2" aria-label="Subtitles">
	<div class="flex flex-wrap items-center gap-2">
		<h3 class="flex-1 {FILE_SECTION_HEADING}">Subtitles</h3>
		<Tooltip.Provider>
			<div class="flex gap-0.5">
				<Tooltip.Root>
					<Tooltip.Trigger>
						{#snippet child({ props })}
							<Button
								{...props}
								size="icon-sm"
								variant="ghost"
								aria-label={SEARCH_LABEL}
								disabled={subs.searching || subs.status !== LoadStatus.READY}
								onclick={() => subs.searchNow()}
							>
								<RefreshCw class={cn(subs.searching && 'animate-spin')} />
							</Button>
						{/snippet}
					</Tooltip.Trigger>
					<Tooltip.Content>{SEARCH_LABEL}</Tooltip.Content>
				</Tooltip.Root>
				<Tooltip.Root>
					<Tooltip.Trigger>
						{#snippet child({ props })}
							<Button
								{...props}
								size="icon-sm"
								variant="ghost"
								aria-label={FIND_LABEL}
								disabled={subs.status !== LoadStatus.READY}
								onclick={() => subs.openPicker()}
							>
								<Search />
							</Button>
						{/snippet}
					</Tooltip.Trigger>
					<Tooltip.Content>{FIND_LABEL}</Tooltip.Content>
				</Tooltip.Root>
			</div>
		</Tooltip.Provider>
	</div>

	{#if subs.status === LoadStatus.LOADING}
		<Skeleton class="h-8 w-full" aria-label="Loading subtitles" />
	{:else if subs.status === LoadStatus.ERROR || !subs.data}
		<div class="flex flex-wrap items-center gap-3" role="alert">
			<p class="text-sm text-destructive">{subs.error}</p>
			<Button size="sm" variant="outline" onclick={() => subs.load()}>Retry</Button>
		</div>
	{:else if subs.data.subtitles.length === 0 && subs.unmet.length === 0}
		<p class="text-sm text-muted-foreground">No subtitles yet.</p>
	{:else}
		<ul class="grid divide-y rounded-lg border" aria-label="Subtitle tracks">
			{#each subs.unmet as c (`${c.language}-${c.forced}`)}
				<li class="flex items-center gap-2 py-1 pr-1 pl-3">
					<span class="text-sm font-medium">{trackLabel(c)}</span>
					<span class="flex-1 text-xs {UNMET_TONES[c.status]}">{coverageLabel(c)}</span>
					<Button
						size="icon-sm"
						variant="ghost"
						class="text-muted-foreground"
						aria-label="Find {trackLabel(c)} subtitles"
						onclick={() => subs.openPicker(c.language, c.forced)}
					>
						<Search />
					</Button>
				</li>
			{/each}
			{#each subs.added as sub (sub.id)}
				{@const sync = syncLabel(sub)}
				<li class="flex items-center gap-2 py-1 pr-1 pl-3">
					<div class="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-0.5">
						<span class="text-sm font-medium">{trackLabel(sub)}</span>
						<span class="text-xs text-muted-foreground">
							{SOURCE_LABELS[sub.source] ?? sub.source}
							{#if sub.score !== undefined}· {sub.score}%{/if}
						</span>
						{#if sync}
							<span class={cn('text-xs', sub.synced ? 'text-primary' : 'text-muted-foreground')}
								>{sync}</span
							>
						{/if}
						{#if sub.path}
							<span
								class="hidden min-w-0 flex-1 truncate text-right font-mono text-xs text-muted-foreground sm:block"
								title={sub.path}>{fileName(sub.path)}</span
							>
						{/if}
					</div>
					<Button
						size="icon-sm"
						variant="ghost"
						class="text-muted-foreground hover:text-destructive"
						aria-label="Remove {trackLabel(sub)} subtitle"
						disabled={subs.removing === sub.id}
						onclick={() => subs.remove(sub.id)}
					>
						<Trash2 />
					</Button>
				</li>
			{/each}
			{#if subs.embedded.length > 0}
				{@const summary = embeddedSummary(subs.embedded, wantedLanguages(subs.data.coverage))}
				<li class="flex min-h-10 items-center gap-2 py-1 pr-1 pl-3">
					<HoverPopover class="w-72 p-3" align="start">
						{#snippet trigger(props)}
							<button
								{...props}
								type="button"
								class="text-sm font-medium underline decoration-muted-foreground decoration-dotted underline-offset-4"
								aria-label="Embedded subtitle languages: {summary}"
							>
								{summary}
							</button>
						{/snippet}
						<ul class="flex flex-wrap gap-x-3 gap-y-1 text-xs" aria-label="Embedded subtitles">
							{#each subs.embedded as { language, count } (language)}
								<li>
									{languageName(language)}{#if count > 1}<span class="text-muted-foreground">
											×{count}</span
										>{/if}
								</li>
							{/each}
						</ul>
					</HoverPopover>
					<span class="text-xs text-muted-foreground">{SOURCE_LABELS[SubtitleSource.EMBEDDED]}</span
					>
				</li>
			{/if}
		</ul>
	{/if}
</section>

<CandidatePicker {subs} />
