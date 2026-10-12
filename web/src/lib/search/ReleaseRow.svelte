<script lang="ts">
	import { fly } from 'svelte/transition';
	import Ban from '@lucide/svelte/icons/ban';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Download from '@lucide/svelte/icons/download';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import Star from '@lucide/svelte/icons/star';
	import { Button } from '$lib/components/ui/button';
	import type { ReleaseDecision } from '$lib/sdk';
	import { formatSize } from './format';
	import { GrabState, isAccepted, type InteractiveSearch } from './interactive-search.svelte';

	let {
		search,
		release,
		best = false
	}: { search: InteractiveSearch; release: ReleaseDecision; best?: boolean } = $props();

	const ENTER_OFFSET_PX = 8;
	const ENTER_DURATION_MS = 300;

	const rejected = $derived(!isAccepted(release));
	const reasons = $derived((release.rejections ?? []).map((r) => r.message).join(' · '));
	const grabState = $derived(search.grabState(release));
</script>

<div
	in:fly={{ y: ENTER_OFFSET_PX, duration: ENTER_DURATION_MS }}
	class={[
		'flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border px-3 py-2 lg:flex-nowrap',
		best && 'border-primary/50 bg-primary/5',
		rejected && 'opacity-70'
	]}
	data-best={best || undefined}
>
	{#if best}
		<Star class="size-4 shrink-0 fill-primary text-primary" aria-label="Best in this quality" />
	{:else}
		<span class="hidden size-4 shrink-0 lg:block"></span>
	{/if}
	<div class={['min-w-0 flex-1 lg:basis-auto', best ? 'basis-[calc(100%-2rem)]' : 'basis-full']}>
		<div
			class={[
				'font-mono text-xs break-all lg:truncate lg:break-normal',
				rejected && 'text-muted-foreground'
			]}
			title={release.name}
		>
			{release.name}
		</div>
		<div class="mt-1 flex flex-wrap items-center gap-1">
			{#each release.formats ?? [] as format (format)}
				<span
					class="rounded px-1.5 py-px text-[10px] {search.isPenalized(format)
						? 'bg-destructive/20 text-destructive'
						: 'bg-primary/15 text-primary'}">{format}</span
				>
			{/each}
			{#if release.freeleech}
				<span class="rounded border border-primary/40 px-1.5 py-px text-[10px] text-primary">
					Freeleech
				</span>
			{/if}
			{#if reasons}
				<span class="text-[11px] text-destructive">{reasons}</span>
			{/if}
		</div>
	</div>
	<div class="flex min-w-0 flex-1 items-center gap-3 text-xs text-muted-foreground lg:contents">
		<span class="text-foreground tabular-nums lg:w-14 lg:text-right" title="Custom format score">
			{release.cfScore}
		</span>
		<span class="whitespace-nowrap tabular-nums lg:w-16 lg:text-right"
			>{formatSize(release.sizeBytes)}</span
		>
		<span class="tabular-nums lg:w-12 lg:text-right" title="Seeders">↑{release.seeders}</span>
		<span class="truncate lg:w-28">{release.indexer}</span>
	</div>
	{#if grabState === GrabState.CHECKING}
		<Button size="sm" variant="outline" disabled>
			<LoaderCircle class="animate-spin" />Checking…
		</Button>
	{:else if grabState === GrabState.DOWNLOADING}
		<Button size="sm" variant="outline" class="border-primary/50 text-primary" disabled>
			<CircleCheck />Downloading
		</Button>
	{:else if grabState === GrabState.REJECTED}
		<Button size="sm" variant="outline" class="border-destructive/50 text-destructive" disabled>
			<Ban />Rejected
		</Button>
	{:else}
		<Button
			size="sm"
			variant={rejected ? 'ghost' : 'default'}
			title={rejected ? 'Grab anyway (override)' : 'Grab'}
			onclick={() => search.grab(release)}
		>
			<Download />{rejected ? 'Override' : 'Grab'}
		</Button>
	{/if}
</div>
