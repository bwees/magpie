<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import { KIND_LABELS } from '$lib/library/library';
	import { LoadStatus } from '$lib/load-status';
	import { formatScore, type ScanReviewManager } from './scan-review.svelte';

	let { review }: { review: ScanReviewManager } = $props();

	const SKELETON_ROWS = 2;
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Review matches</Card.Title>
		<Card.Description>
			Folders the scan couldn't match with certainty. Pick the title, search TMDB for another, or
			ignore the folder.
		</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-3">
		{#if review.status === LoadStatus.LOADING}
			<div class="grid gap-2" aria-busy="true" aria-label="Loading folders to review">
				{#each { length: SKELETON_ROWS }, i (i)}
					<Skeleton class="h-28 w-full rounded-lg" />
				{/each}
			</div>
		{:else if review.status === LoadStatus.ERROR}
			<div class="flex flex-wrap items-center gap-2" role="alert">
				<span class="text-sm text-destructive">{review.error}</span>
				<Button size="sm" variant="outline" onclick={() => review.load()}>Retry</Button>
			</div>
		{:else if review.rows.length === 0}
			<p class="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">
				Nothing to review
			</p>
		{:else}
			<ul class="grid gap-3" aria-label="Folders to review">
				{#each review.rows as row (row.id)}
					{@const candidates = review.candidates(row)}
					{@const search = review.searches.get(row.id)}
					<li class="grid gap-3 rounded-lg border p-3">
						<div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
							<div class="grid min-w-0 gap-1">
								<div class="flex flex-wrap items-center gap-2">
									<Badge variant="secondary">{KIND_LABELS[row.kind] ?? row.kind}</Badge>
									<span class="text-sm font-medium">
										{row.parsedTitle}{row.parsedYear ? ` (${row.parsedYear})` : ''}
									</span>
								</div>
								<code class="font-mono text-xs break-all text-muted-foreground"
									>{row.folderPath}</code
								>
							</div>
							<Button
								size="sm"
								variant="ghost"
								class="self-start"
								disabled={review.busy.has(row.id)}
								onclick={() => review.ignore(row)}
							>
								Ignore
							</Button>
						</div>
						{#if candidates.length === 0}
							<p class="text-sm text-muted-foreground">
								{search?.results ? 'TMDB found nothing of this kind.' : 'TMDB had no likely match.'}
							</p>
						{:else}
							<ul class="grid gap-2" aria-label="Candidates for {row.folderPath}">
								{#each candidates as candidate (candidate.tmdbId)}
									<li class="flex items-center gap-3 rounded-md bg-muted/40 p-2">
										<PosterImage src={candidate.posterUrl} alt="" class="w-10 shrink-0" />
										<div class="grid min-w-0 flex-1 gap-0.5">
											<span class="truncate text-sm font-medium">
												{candidate.name}{candidate.year ? ` (${candidate.year})` : ''}
											</span>
											{#if candidate.overview}
												<span class="line-clamp-2 text-xs text-muted-foreground">
													{candidate.overview}
												</span>
											{/if}
										</div>
										{#if candidate.score !== undefined}
											<Badge variant="outline" class="hidden shrink-0 tabular-nums sm:inline-flex">
												{formatScore(candidate.score)}
											</Badge>
										{/if}
										<Button
											size="sm"
											variant="outline"
											class="shrink-0"
											disabled={review.busy.has(row.id)}
											onclick={() => review.match(row, candidate)}
										>
											Match
										</Button>
									</li>
								{/each}
							</ul>
						{/if}
						<form
							class="flex gap-2"
							onsubmit={(event) => {
								event.preventDefault();
								void review.search(row);
							}}
						>
							<Input
								class="flex-1"
								aria-label="Search TMDB for {row.folderPath}"
								value={review.query(row)}
								oninput={(event) => review.setQuery(row, event.currentTarget.value)}
							/>
							<Button
								type="submit"
								size="sm"
								variant="outline"
								class="h-9"
								disabled={search?.searching}
							>
								<Search />Search
							</Button>
						</form>
						{#if review.rowErrors.has(row.id)}
							<p class="text-sm text-destructive" role="alert">{review.rowErrors.get(row.id)}</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
</Card.Root>
