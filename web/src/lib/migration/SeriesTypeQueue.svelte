<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import DecisionBadge from './DecisionBadge.svelte';
	import { ReviewQueue, SeriesType, SeriesTypeDecision } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const reviews = $derived(wizard.state?.reviews?.seriesTypes ?? []);

	const TYPE_NOTES: Record<SeriesType, string> = {
		[SeriesType.DAILY]: 'Daily show. Magpie imports it as a standard series, numbered by TMDB.',
		[SeriesType.ANIME]:
			'Anime. Magpie has no absolute numbering, so releases may not match its episodes.'
	};
	const DECISION_LABELS: Partial<Record<SeriesTypeDecision, string>> = {
		[SeriesTypeDecision.IMPORT]: 'Importing',
		[SeriesTypeDecision.SKIP]: 'Skipped'
	};

	function decide(id: string, seriesType: SeriesTypeDecision) {
		void wizard.decide({ queue: ReviewQueue.SERIES_TYPES, id, seriesType });
	}
</script>

{#if reviews.length === 0}
	<p class="text-sm text-muted-foreground">No daily or anime series.</p>
{:else}
	<ul class="grid gap-3">
		{#each reviews as review (review.id)}
			<li class="flex flex-wrap items-center gap-3 rounded-lg border p-3">
				<div class="grid min-w-0 flex-1 gap-0.5">
					<span class="flex flex-wrap items-center gap-2 font-medium">
						{review.name}
						<DecisionBadge label={DECISION_LABELS[review.decision as SeriesTypeDecision]} />
					</span>
					<span class="text-sm text-muted-foreground">{TYPE_NOTES[review.type as SeriesType]}</span>
				</div>
				<div class="flex gap-2">
					<Button
						size="sm"
						variant="ghost"
						disabled={wizard.busy}
						onclick={() => decide(review.id, SeriesTypeDecision.SKIP)}>Skip</Button
					>
					<Button
						size="sm"
						variant="secondary"
						disabled={wizard.busy}
						onclick={() => decide(review.id, SeriesTypeDecision.IMPORT)}>Import as standard</Button
					>
				</div>
			</li>
		{/each}
	</ul>
{/if}
