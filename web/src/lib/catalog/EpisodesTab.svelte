<script lang="ts">
	import Search from '@lucide/svelte/icons/search';
	import TextSearch from '@lucide/svelte/icons/text-search';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/activity/activity-source';
	import { STAGE_LABELS, Stage } from '$lib/activity/pipeline';
	import type { TitlePipeline } from '$lib/activity/title-pipeline.svelte';
	import type { EpisodeResponse, SeasonResponse, TitleResponse } from '$lib/sdk';
	import { AutomaticSearch } from '$lib/search/automatic-search.svelte';
	import InteractiveSearchDialog from '$lib/search/InteractiveSearchDialog.svelte';
	import type { SearchTarget } from '$lib/search/search-target';
	import {
		airDateLabel,
		AVAILABLE_UNKNOWN,
		episodeCode,
		newestSeasonsFirst,
		seasonLabel
	} from './episodes';
	import type { TitlePage } from './title-page.svelte';
	import EpisodeFilesDialog from '$lib/library/EpisodeFilesDialog.svelte';
	import type { OpenEpisode, TitleFiles } from '$lib/library/title-files.svelte';

	let {
		page,
		pipeline,
		files,
		manageFiles
	}: { page: TitlePage; pipeline: TitlePipeline; files: TitleFiles; manageFiles: boolean } =
		$props();

	const SEARCH_LABEL = 'Search episode';
	const INTERACTIVE_LABEL = 'Interactive search';
	/** Shown for a released episode nobody wants, which has no pipeline row. */
	const NO_STAGE = '—';

	const automatic = new AutomaticSearch();
	let interactiveTarget = $state.raw<SearchTarget>();
	let searching = $state(false);

	function searchInteractively(target: SearchTarget): void {
		interactiveTarget = target;
		searching = true;
	}

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

	function episodeFiles(episode: EpisodeResponse, season: number): OpenEpisode {
		const code = episodeCode(season, episode.number);
		return { id: episode.id, label: episode.name ? `${code} · ${episode.name}` : code };
	}

	/** The row opens the episode's files, so its controls keep their clicks to themselves. */
	function stopRowClick(event: MouseEvent): void {
		event.stopPropagation();
	}

	function seasonTarget(title: TitleResponse, season: SeasonResponse): SearchTarget {
		return {
			titleId: title.id,
			episodeIds: season.episodes.map((episode) => episode.id),
			label: `${title.name} ${seasonLabel(season.number)}`
		};
	}
</script>

{#if page.title}
	<Tooltip.Provider>
		<div class="grid gap-4">
			{#each newestSeasonsFirst(page.title.seasons) as season (season.number)}
				{@const label = seasonLabel(season.number)}
				<Card.Root class="gap-0 py-0">
					<Card.Header class="flex flex-row items-center justify-between gap-3 border-b py-3">
						<div class="flex items-baseline gap-3">
							<Card.Title class="text-base">{label}</Card.Title>
							<span class="text-xs text-muted-foreground tabular-nums">
								{pipeline.status === LoadStatus.READY
									? pipeline.availableCount(season.episodes.map((episode) => episode.id))
									: AVAILABLE_UNKNOWN} / {season.episodes.length}
							</span>
						</div>
						<div class="flex items-center gap-3">
							{#if manageFiles && season.episodes.length > 0}
								{@const target = seasonTarget(page.title, season)}
								<Tooltip.Root>
									<Tooltip.Trigger>
										{#snippet child({ props })}
											<Button
												{...props}
												size="icon-sm"
												variant="ghost"
												aria-label="{INTERACTIVE_LABEL} {label}"
												onclick={() => searchInteractively(target)}
											>
												<TextSearch />
											</Button>
										{/snippet}
									</Tooltip.Trigger>
									<Tooltip.Content>{INTERACTIVE_LABEL} {label}</Tooltip.Content>
								</Tooltip.Root>
							{/if}
							<label class="flex items-center gap-2 text-xs text-muted-foreground">
								Monitored
								<Switch
									checked={season.monitored}
									aria-label="Monitor {label}"
									onCheckedChange={(checked) => page.setSeasonMonitored(season.number, checked)}
								/>
							</label>
						</div>
					</Card.Header>
					{#if season.episodes.length === 0}
						<p class="px-4 py-3 text-sm text-muted-foreground">No episodes announced yet.</p>
					{:else}
						<Table.Root>
							<Table.Body>
								{#each season.episodes as episode (episode.id)}
									{@const showFiles = () => files.showEpisode(episodeFiles(episode, season.number))}
									<Table.Row class="cursor-pointer" onclick={showFiles}>
										<Table.Cell class="w-10 pl-4 text-muted-foreground tabular-nums">
											{episode.number}
										</Table.Cell>
										<Table.Cell class="max-w-0 w-full">
											<button
												type="button"
												class="block w-full truncate text-left font-medium"
												aria-label="Show files of {episodeCode(season.number, episode.number)}"
												onclick={(event) => {
													event.stopPropagation();
													showFiles();
												}}>{episode.name || 'TBA'}</button
											>
										</Table.Cell>
										<Table.Cell
											class="hidden w-32 text-right text-muted-foreground tabular-nums sm:table-cell"
										>
											{airDateLabel(episode.airDate)}
										</Table.Cell>
										<Table.Cell class="w-28 text-center">
											{@const row = pipeline.byEpisode[episode.id]}
											{#if pipeline.status === LoadStatus.LOADING}
												<Skeleton class="mx-auto h-5 w-20" />
											{:else if row}
												<Badge
													variant={row.failed
														? 'destructive'
														: row.stage === Stage.AVAILABLE
															? 'secondary'
															: 'default'}
													title={row.detail || undefined}
												>
													{STAGE_LABELS[row.stage as Stage] ?? row.stage}
												</Badge>
											{:else if !episode.released}
												<Badge variant="outline">Unaired</Badge>
											{:else}
												<span class="text-muted-foreground">{NO_STAGE}</span>
											{/if}
										</Table.Cell>
										<Table.Cell onclick={stopRowClick}>
											<Switch
												checked={episode.monitored}
												aria-label="Monitor episode {episode.number}"
												onCheckedChange={(checked) => page.setEpisodeMonitored(episode.id, checked)}
											/>
										</Table.Cell>
										{#if manageFiles}
											{@const target = episodeTarget(page.title, episode, season.number)}
											{@const code = episodeCode(season.number, episode.number)}
											<Table.Cell class="pr-4" onclick={stopRowClick}>
												<div class="flex items-center gap-1">
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
													<Tooltip.Root>
														<Tooltip.Trigger>
															{#snippet child({ props })}
																<Button
																	{...props}
																	size="icon-sm"
																	variant="ghost"
																	aria-label="{INTERACTIVE_LABEL} {code}"
																	onclick={() => searchInteractively(target)}
																>
																	<TextSearch />
																</Button>
															{/snippet}
														</Tooltip.Trigger>
														<Tooltip.Content>{INTERACTIVE_LABEL}</Tooltip.Content>
													</Tooltip.Root>
												</div>
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

{#if interactiveTarget}
	<InteractiveSearchDialog bind:open={searching} target={interactiveTarget} />
{/if}

<EpisodeFilesDialog {files} />
