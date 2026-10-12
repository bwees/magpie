<script lang="ts">
	import { onMount } from 'svelte';
	import BlockState from '$lib/components/BlockState.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { titlePath } from '$lib/catalog/images';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import { getLive } from '$lib/live/live-client.svelte';
	import { CardState } from '$lib/requests/search-requests.svelte';
	import { scopeLabel } from '$lib/requests/requests';
	import { getShell } from '$lib/shell/shell-state.svelte';
	import { MyRequests } from './my-requests.svelte';
	import { Trending } from './trending.svelte';

	let { userId }: { userId: string } = $props();

	const SKELETON_ROWS = 3;
	const SKELETON_POSTERS = 6;

	const shell = getShell();
	const live = getLive();
	const requests = shell.searchRequests;
	const mine = $derived(new MyRequests(userId));
	const trending = new Trending();

	onMount(() => {
		void trending.load();
		void requests.load();
	});

	$effect(() => {
		void mine.load();
		return mine.watch(live);
	});
</script>

<div class="flex flex-col gap-8">
	<section aria-labelledby="my-requests">
		<h2 id="my-requests" class="mb-3 text-sm font-semibold">Your requests</h2>
		<BlockState
			block={mine.block}
			label="your requests"
			empty="You haven't requested anything yet; pick a title below or search for one."
			onRetry={() => mine.load()}
		>
			{#snippet skeleton()}
				<div class="flex flex-col gap-2">
					{#each { length: SKELETON_ROWS }, i (i)}
						<Skeleton class="h-16 w-full rounded-lg" />
					{/each}
				</div>
			{/snippet}
			<ul class="divide-y rounded-xl border">
				{#each mine.items as request (request.id)}
					{@const media = mine.mediaOf(request)}
					{@const badge = mine.badge(request)}
					<li class="flex items-center gap-3 px-3 py-2">
						<PosterImage src={media?.posterUrl} alt="" class="w-10 shrink-0" />
						<div class="min-w-0 flex-1">
							{#if request.titleId}
								<a
									href={titlePath(request.titleId)}
									class="block truncate text-sm font-medium hover:underline"
								>
									{mine.nameOf(request)}
								</a>
							{:else}
								<span class="block truncate text-sm font-medium">{mine.nameOf(request)}</span>
							{/if}
							<span class="block truncate text-xs text-muted-foreground">
								{[scopeLabel(request.scope, request.seasons), badge.detail]
									.filter(Boolean)
									.join(' · ')}
							</span>
						</div>
						<Badge variant={badge.failed ? 'destructive' : 'secondary'} class="shrink-0">
							{badge.label}
						</Badge>
					</li>
				{/each}
			</ul>
		</BlockState>
	</section>

	<section aria-labelledby="trending">
		<h2 id="trending" class="mb-3 text-sm font-semibold">Trending this week</h2>
		<BlockState
			block={trending.block}
			label="trending titles"
			empty="TMDB lists nothing trending right now."
			onRetry={() => trending.load()}
		>
			{#snippet skeleton()}
				<div class="flex gap-3 overflow-hidden">
					{#each { length: SKELETON_POSTERS }, i (i)}
						<Skeleton class="aspect-[2/3] w-32 shrink-0 rounded-md" />
					{/each}
				</div>
			{/snippet}
			<ul class="-mx-4 flex gap-3 overflow-x-auto px-4 pb-2 md:mx-0 md:px-0">
				{#each trending.items as item (`${item.kind}-${item.tmdbId}`)}
					{@const state = requests.stateOf(item)}
					<li class="flex w-28 shrink-0 flex-col gap-1 sm:w-32">
						<PosterImage src={item.posterUrl} alt={item.name} />
						<span class="truncate text-xs font-medium" title={item.name}>{item.name}</span>
						<span class="text-[11px] text-muted-foreground">{item.year || '—'}</span>
						{#if state === CardState.IN_LIBRARY && item.titleId}
							<Badge href={titlePath(item.titleId)} variant="secondary" class="w-full"
								>In library</Badge
							>
						{:else if state === CardState.REQUESTED}
							<Badge variant="outline" class="w-full">Requested</Badge>
						{:else}
							<Button
								size="xs"
								class="w-full"
								disabled={requests.isSending(item)}
								onclick={() => requests.request(item)}
							>
								Request
							</Button>
						{/if}
					</li>
				{/each}
			</ul>
		</BlockState>
	</section>
</div>
