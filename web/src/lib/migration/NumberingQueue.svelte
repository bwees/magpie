<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import type { MigrationSeasonCount } from '$lib/sdk';
	import DecisionBadge from './DecisionBadge.svelte';
	import { NumberingDecision, ReviewQueue, scorePercent } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const reviews = $derived(wizard.state?.reviews?.numbering ?? []);

	function seasons(counts: MigrationSeasonCount[]): string {
		return counts.map((c) => `S${c.season}: ${c.episodes}`).join(' · ') || 'none';
	}

	const DECISION_LABELS: Partial<Record<NumberingDecision, string>> = {
		[NumberingDecision.AIRED]: 'Aired order',
		[NumberingDecision.GROUP]: 'Episode group'
	};
</script>

{#if reviews.length === 0}
	<p class="text-sm text-muted-foreground">Every series numbers its episodes as TMDB airs them.</p>
{:else}
	<ul class="grid gap-3">
		{#each reviews as review (review.id)}
			<li class="grid gap-3 rounded-lg border p-3">
				<div class="flex flex-wrap items-center gap-2">
					<span class="font-medium">{review.name}</span>
					<DecisionBadge label={DECISION_LABELS[review.decision as NumberingDecision]} />
				</div>
				<dl class="grid gap-1 text-sm sm:grid-cols-2">
					<div>
						<dt class="text-xs text-muted-foreground">Sonarr seasons</dt>
						<dd class="tabular-nums">{seasons(review.sonarrSeasons)}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">TMDB aired order</dt>
						<dd class="tabular-nums">{seasons(review.tmdbSeasons)}</dd>
					</div>
				</dl>
				<ul class="grid gap-1.5">
					{#each review.groups as group (group.id)}
						{@const active =
							review.decision === NumberingDecision.GROUP && review.group === group.id}
						<li class="flex flex-wrap items-center gap-2 text-sm">
							<span class="min-w-0 flex-1">
								{group.name}
								<span class="text-muted-foreground"
									>· {scorePercent(group.score)}% of seasons match</span
								>
								{#if group.id === review.suggested}<Badge variant="outline" class="ml-1"
										>Suggested</Badge
									>{/if}
							</span>
							<Button
								size="sm"
								variant={active ? 'default' : 'secondary'}
								disabled={wizard.busy}
								onclick={() =>
									wizard.decide({
										queue: ReviewQueue.NUMBERING,
										id: review.id,
										numbering: NumberingDecision.GROUP,
										group: group.id
									})}
							>
								{active ? 'Using' : 'Use'}
							</Button>
						</li>
					{/each}
				</ul>
				<div class="flex justify-end">
					<Button
						size="sm"
						variant={review.decision === NumberingDecision.AIRED ? 'default' : 'ghost'}
						disabled={wizard.busy}
						onclick={() =>
							wizard.decide({
								queue: ReviewQueue.NUMBERING,
								id: review.id,
								numbering: NumberingDecision.AIRED
							})}
					>
						Keep aired order
					</Button>
				</div>
			</li>
		{/each}
	</ul>
{/if}
