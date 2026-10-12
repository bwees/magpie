<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Input } from '$lib/components/ui/input';
	import type { MigrationCandidate } from '$lib/sdk';
	import DecisionBadge from './DecisionBadge.svelte';
	import { APP_LABELS, MediaKind, ReviewQueue, TitleDecision, type ArrApp } from './migration';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const titles = $derived(wizard.state?.reviews?.titles ?? []);

	const DECISION_LABELS: Partial<Record<TitleDecision, string>> = {
		[TitleDecision.MATCH]: 'Matched',
		[TitleDecision.SPLIT]: 'Split',
		[TitleDecision.SKIP]: 'Skipped'
	};

	function candidatesOf(id: string, offered: MigrationCandidate[]): MigrationCandidate[] {
		const found = wizard.search(id).results;
		return [...offered, ...found.filter((f) => !offered.some((o) => o.tmdb === f.tmdb))];
	}

	function decide(id: string, title: TitleDecision, tmdb: number[] = []) {
		void wizard.decide({ queue: ReviewQueue.TITLES, id, title, tmdb });
	}
</script>

{#if titles.length === 0}
	<p class="text-sm text-muted-foreground">Every title matched TMDB by its ID.</p>
{:else}
	<ul class="grid gap-3">
		{#each titles as review (review.id)}
			{@const search = wizard.search(review.id)}
			{@const series = review.kind === MediaKind.SERIES}
			<li class="grid gap-3 rounded-lg border p-3">
				<div class="flex flex-wrap items-center gap-2">
					<span class="font-medium">{review.name}</span>
					<span class="text-sm text-muted-foreground"
						>{review.year || ''} · {APP_LABELS[review.app as ArrApp]}</span
					>
					<DecisionBadge label={DECISION_LABELS[review.decision as TitleDecision]} />
				</div>
				<form
					class="flex gap-2"
					onsubmit={(event) => {
						event.preventDefault();
						void wizard.runSearch(review.id, review.kind);
					}}
				>
					<Input
						class="min-w-0"
						placeholder="Search TMDB"
						bind:value={search.query}
						aria-label="Search TMDB for {review.name}"
					/>
					<Button type="submit" variant="outline" disabled={search.searching}
						>{search.searching ? 'Searching…' : 'Search'}</Button
					>
				</form>
				{#if search.error}<p class="text-sm text-destructive">{search.error}</p>{/if}
				<ul class="grid gap-1.5">
					{#each candidatesOf(review.id, review.candidates) as candidate (candidate.tmdb)}
						<li class="flex flex-wrap items-center gap-2 text-sm">
							{#if series}
								<Checkbox
									checked={search.picked.includes(candidate.tmdb)}
									onCheckedChange={() => wizard.togglePick(review.id, candidate.tmdb)}
									aria-label="Pick {candidate.name} for a split"
								/>
							{/if}
							<span class="min-w-0 flex-1"
								>{candidate.name}
								<span class="text-muted-foreground"
									>{candidate.year || ''} · TMDB {candidate.tmdb}</span
								></span
							>
							<Button
								size="sm"
								variant="secondary"
								disabled={wizard.busy}
								onclick={() => decide(review.id, TitleDecision.MATCH, [candidate.tmdb])}
								>Match</Button
							>
						</li>
					{:else}
						<li class="text-sm text-muted-foreground">No candidates yet. Search TMDB above.</li>
					{/each}
				</ul>
				<div class="flex flex-wrap justify-end gap-2">
					{#if series && search.picked.length > 1}
						<Button
							size="sm"
							variant="outline"
							disabled={wizard.busy}
							onclick={() => decide(review.id, TitleDecision.SPLIT, search.picked)}
						>
							Split into {search.picked.length} titles
						</Button>
					{/if}
					<Button
						size="sm"
						variant="ghost"
						disabled={wizard.busy}
						onclick={() => decide(review.id, TitleDecision.SKIP)}>Skip</Button
					>
				</div>
			</li>
		{/each}
	</ul>
{/if}
