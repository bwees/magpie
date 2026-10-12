<script lang="ts">
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { POSTER_GRID, SKELETON_POSTERS } from './layout';

	let { count = SKELETON_POSTERS }: { count?: number } = $props();
</script>

<div class={POSTER_GRID}>
	{#each { length: count }, i (i)}
		<div class="flex flex-col gap-2">
			<Skeleton class="aspect-[2/3] w-full rounded-md" />
			<Skeleton class="h-3 w-3/4" />
		</div>
	{/each}
</div>
