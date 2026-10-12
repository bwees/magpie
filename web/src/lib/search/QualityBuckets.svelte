<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import type { ReleaseDecision } from '$lib/sdk';
	import ReleaseRow from './ReleaseRow.svelte';
	import type { InteractiveSearch } from './interactive-search.svelte';

	let { search }: { search: InteractiveSearch } = $props();

	const emptyText = $derived(search.completed ? 'None found' : 'Nothing yet…');
</script>

{#snippet section(name: string, releases: ReleaseDecision[], meetsCutoff: boolean, best?: string)}
	<section>
		<div class="mb-2 flex items-center gap-2">
			<h3 class="text-sm font-semibold">{name}</h3>
			<span class="text-xs text-muted-foreground tabular-nums">{releases.length}</span>
			{#if meetsCutoff}
				<Badge class="h-5">Meets cutoff</Badge>
			{/if}
		</div>
		{#if releases.length === 0}
			<p class="rounded-lg border border-dashed px-4 py-3 text-xs text-muted-foreground">
				{emptyText}
			</p>
		{:else}
			<div class="flex flex-col gap-1.5">
				{#each releases as release (release.release)}
					<ReleaseRow {search} {release} best={release.release === best} />
				{/each}
			</div>
		{/if}
	</section>
{/snippet}

<div class="flex flex-col gap-6">
	{#each search.buckets as bucket (bucket.name)}
		{@render section(bucket.name, bucket.releases, bucket.meetsCutoff, bucket.best)}
	{/each}
	{@render section('Rejected', search.rejected, false)}
</div>
