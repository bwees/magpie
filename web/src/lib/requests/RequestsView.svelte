<script lang="ts">
	import { onMount } from 'svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { ProfileOptions } from '$lib/settings/quality/profile-options.svelte';
	import { cn } from '$lib/utils';
	import DecisionDialogs from './DecisionDialogs.svelte';
	import RequestRow from './RequestRow.svelte';
	import { FILTER_LABELS, STATUS_FILTERS } from './requests';
	import { RequestsPage, type Viewer } from './requests-page.svelte';

	let { viewer }: { viewer: Viewer } = $props();

	const SKELETON_ROWS = ['first', 'second', 'third', 'fourth'];

	// svelte-ignore state_referenced_locally
	const page = new RequestsPage(viewer);
	const profiles = new ProfileOptions();
	const live = getLive();

	onMount(() => {
		void page.load();
		void profiles.load();
		return page.watch(live);
	});
</script>

<div class="flex flex-col gap-4">
	<div
		class="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 md:mx-0 md:px-0"
		role="tablist"
		aria-label="Request status"
	>
		{#each STATUS_FILTERS as filter (filter)}
			{@const active = page.filter === filter}
			<button
				type="button"
				role="tab"
				aria-selected={active}
				class={cn(
					'flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1 text-sm transition-colors',
					active
						? 'border-primary bg-primary text-primary-foreground'
						: 'text-muted-foreground hover:bg-muted hover:text-foreground'
				)}
				onclick={() => page.select(filter)}
			>
				{FILTER_LABELS[filter]}
				<span class={cn('text-xs tabular-nums', active ? 'opacity-80' : 'opacity-70')}>
					{page.count(filter)}
				</span>
			</button>
		{/each}
	</div>

	{#if page.status === LoadStatus.LOADING}
		<div class="flex flex-col gap-3" role="status" aria-label="Loading requests">
			{#each SKELETON_ROWS as key (key)}
				<Skeleton class="h-20 w-full" />
			{/each}
		</div>
	{:else if page.status === LoadStatus.ERROR}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Could not load requests</Alert.Title>
			<Alert.Description>
				<p>{page.error}</p>
				<Button variant="outline" size="sm" class="mt-2" onclick={() => page.load()}>Retry</Button>
			</Alert.Description>
		</Alert.Root>
	{:else if page.empty}
		<p class="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
			Nothing here.
		</p>
	{:else}
		<ul class="divide-y rounded-lg border bg-card px-4">
			{#each page.items as request (request.id)}
				<RequestRow {request} {page} profileName={profiles.name(request.profileId)} />
			{/each}
		</ul>
	{/if}
</div>

<DecisionDialogs {page} />
