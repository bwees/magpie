<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import type { EpisodeResponse, TitleResponse } from '$lib/sdk';
	import { AutomaticSearch, type SearchTarget } from '$lib/search/automatic-search.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import { airDateLabel, AVAILABLE_UNKNOWN, episodeCode, seasonLabel } from './episodes';
	import type { TitlePage } from './title-page.svelte';

	let { page }: { page: TitlePage } = $props();

	const SEARCH_LABEL = 'Search episode';

	const session = getSession();
	const automatic = new AutomaticSearch();

	function episodeTarget(
		title: TitleResponse,
		episode: EpisodeResponse,
		season: number
	): SearchTarget {
		return {
			titleId: title.id,
			episodeIds: [episode.id],
			label: `${title.name} ${episodeCode(season, episode.number)}`
		};
	}
</script>

{#if page.title}
	<Tooltip.Provider>
		<div class="grid gap-4">
			{#each page.title.seasons as season (season.number)}
				{@const label = seasonLabel(season.number)}
				<Card.Root class="gap-0 py-0">
					<Card.Header class="flex flex-row items-center justify-between gap-3 border-b py-3">
						<div class="flex items-baseline gap-3">
							<Card.Title class="text-base">{label}</Card.Title>
							<span class="text-xs text-muted-foreground tabular-nums">
								{AVAILABLE_UNKNOWN} / {season.episodes.length}
							</span>
						</div>
						<label class="flex items-center gap-2 text-xs text-muted-foreground">
							Monitored
							<Switch
								checked={season.monitored}
								aria-label="Monitor {label}"
								onCheckedChange={(checked) => page.setSeasonMonitored(season.number, checked)}
							/>
						</label>
					</Card.Header>
					{#if season.episodes.length === 0}
						<p class="px-4 py-3 text-sm text-muted-foreground">No episodes announced yet.</p>
					{:else}
						<Table.Root>
							<Table.Body>
								{#each season.episodes as episode (episode.id)}
									<Table.Row>
										<Table.Cell class="w-10 pl-4 text-muted-foreground tabular-nums">
											{episode.number}
										</Table.Cell>
										<Table.Cell class="max-w-0 w-full">
											<span class="block truncate font-medium">{episode.name || 'TBA'}</span>
										</Table.Cell>
										<Table.Cell
											class="hidden w-32 text-right text-muted-foreground tabular-nums sm:table-cell"
										>
											{airDateLabel(episode.airDate)}
										</Table.Cell>
										<Table.Cell class="w-24 text-center">
											{#if episode.released}
												<Badge variant="secondary">Aired</Badge>
											{:else}
												<Badge variant="outline">Unaired</Badge>
											{/if}
										</Table.Cell>
										<Table.Cell>
											<Switch
												checked={episode.monitored}
												aria-label="Monitor episode {episode.number}"
												onCheckedChange={(checked) => page.setEpisodeMonitored(episode.id, checked)}
											/>
										</Table.Cell>
										{#if session.isAdmin}
											{@const target = episodeTarget(page.title, episode, season.number)}
											<Table.Cell class="pr-4">
												<Tooltip.Root>
													<Tooltip.Trigger>
														{#snippet child({ props })}
															<Button
																{...props}
																size="icon-sm"
																variant="ghost"
																aria-label={SEARCH_LABEL}
																disabled={automatic.isStarting(target)}
																onclick={() => automatic.search(target)}
															>
																<Search />
															</Button>
														{/snippet}
													</Tooltip.Trigger>
													<Tooltip.Content>{SEARCH_LABEL}</Tooltip.Content>
												</Tooltip.Root>
											</Table.Cell>
										{/if}
									</Table.Row>
								{/each}
							</Table.Body>
						</Table.Root>
					{/if}
				</Card.Root>
			{:else}
				<p class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
					TMDB lists no seasons for this series yet.
				</p>
			{/each}
		</div>
	</Tooltip.Provider>
{/if}
