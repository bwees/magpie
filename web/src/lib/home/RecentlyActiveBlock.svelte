<script lang="ts">
	import BlockState from '$lib/components/BlockState.svelte';
	import { POSTER_GRID } from '$lib/catalog/layout';
	import PosterCard from '$lib/catalog/PosterCard.svelte';
	import PosterGridSkeleton from '$lib/catalog/PosterGridSkeleton.svelte';
	import { Route } from '$lib/navigation/nav';
	import type { RecentlyActive } from './recently-active.svelte';

	let { recent }: { recent: RecentlyActive } = $props();

	const UNKNOWN_TITLE = 'Unknown title';
</script>

<section aria-labelledby="recently-active">
	<div class="mb-3 flex items-baseline justify-between">
		<h2 id="recently-active" class="text-sm font-semibold">Recently active</h2>
		<a href={Route.MOVIES} class="text-xs text-muted-foreground hover:underline">View library →</a>
	</div>
	<BlockState
		block={recent.block}
		label="recent activity"
		empty="Nothing has happened in your library yet."
		onRetry={() => recent.load()}
	>
		{#snippet skeleton()}
			<PosterGridSkeleton />
		{/snippet}
		<div class={POSTER_GRID}>
			{#each recent.rows as row (row.titleId)}
				{@const title = recent.directory.get(row.titleId)}
				<PosterCard
					titleId={row.titleId}
					name={title?.name ?? UNKNOWN_TITLE}
					year={title?.year}
					kind={title?.kind}
					poster={title?.images.poster}
					stage={row}
					percent={recent.progress.of(row)}
				/>
			{/each}
		</div>
	</BlockState>
</section>
