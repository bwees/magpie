<script lang="ts">
	import { untrack } from 'svelte';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import * as Select from '$lib/components/ui/select';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import ProfileSelect from '$lib/settings/quality/ProfileSelect.svelte';
	import { TitleExclusion } from '$lib/transcode/title-exclusion.svelte';
	import WantedLanguagesEditor from './WantedLanguagesEditor.svelte';
	import type { TitlePage } from './title-page.svelte';
	import {
		AIRED_ORDER,
		EPISODE_GROUP_TYPE_LABELS,
		EpisodeOrder,
		LanguageOverride,
		LanguageSource
	} from './title-settings.svelte';

	let { page, onRemoved }: { page: TitlePage; onRemoved: () => void } = $props();

	const order = $derived(new EpisodeOrder(page));
	const languages = $derived(new LanguageOverride(page));
	const exclusion = $derived(new TitleExclusion(page.id));

	const LANGUAGE_SOURCE_LABELS: Record<LanguageSource, string> = {
		[LanguageSource.GLOBAL]: 'Use the global list',
		[LanguageSource.CUSTOM]: 'Custom list for this title'
	};

	$effect(() => {
		const [currentOrder, currentLanguages, currentExclusion] = [order, languages, exclusion];
		untrack(() => {
			if (page.isSeries) currentOrder.load();
			currentLanguages.load();
			currentExclusion.load();
		});
	});

	async function confirmRemove() {
		if (await page.remove()) {
			onRemoved();
		}
	}
</script>

{#if page.title}
	<div class="grid max-w-2xl gap-6">
		<div class="grid gap-1.5">
			<label for="title-profile" class="text-sm font-medium">Quality profile</label>
			<ProfileSelect
				id="title-profile"
				value={page.title.profileId}
				onChange={(profileId) => page.setProfile(profileId)}
			/>
		</div>
		<div class="flex items-center justify-between gap-4">
			<div>
				<label for="title-monitored" class="text-sm font-medium">Monitored</label>
				<p class="text-xs text-muted-foreground">
					Search automatically for missing files and upgrades
				</p>
			</div>
			<Switch
				id="title-monitored"
				checked={page.title.monitored}
				onCheckedChange={(checked) => page.setMonitored(checked)}
			/>
		</div>

		{#if page.isSeries}
			<div class="flex items-center justify-between gap-4">
				<div>
					<label for="title-future-seasons" class="text-sm font-medium">Monitor new seasons</label>
					<p class="text-xs text-muted-foreground">Seasons TMDB announces later start monitored</p>
				</div>
				<Switch
					id="title-future-seasons"
					checked={page.title.monitorFutureSeasons}
					onCheckedChange={(checked) => page.setMonitorFutureSeasons(checked)}
				/>
			</div>

			<div class="grid gap-2">
				<span id="episode-order-label" class="text-sm font-medium">Episode order</span>
				{#if order.status === LoadStatus.LOADING}
					<Skeleton class="h-9 w-full sm:w-80" />
				{:else if order.status === LoadStatus.ERROR}
					<div class="flex flex-wrap items-center gap-3">
						<p class="text-sm text-destructive">{order.error}</p>
						<Button size="sm" variant="outline" onclick={() => order.load()}>Retry</Button>
					</div>
				{:else}
					<Select.Root
						type="single"
						value={order.selected}
						onValueChange={(groupId) => order.select(groupId)}
						disabled={order.saving}
					>
						<Select.Trigger class="w-full sm:w-80" aria-labelledby="episode-order-label">
							{order.saving ? 'Renumbering…' : order.selectedLabel}
						</Select.Trigger>
						<Select.Content>
							<Select.Item value={AIRED_ORDER} label="Aired order (TMDB default)" />
							{#each order.groups as group (group.id)}
								<Select.Item value={group.id} label={group.name}>
									<span class="flex min-w-0 flex-col">
										<span class="truncate">{group.name}</span>
										<span class="text-xs text-muted-foreground">
											{EPISODE_GROUP_TYPE_LABELS[group.type] ?? group.type} · {group.episodeCount}
											episodes in {group.groupCount} seasons
										</span>
									</span>
								</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				{/if}
				<p class="text-xs text-muted-foreground">
					Magpie names episode files by this order. Pick the same episode group in the series'
					Jellyfin metadata settings, or Jellyfin will match the wrong episodes.
				</p>
			</div>
		{/if}

		<div class="flex items-center justify-between gap-4">
			<div>
				<label for="title-transcode-excluded" class="text-sm font-medium"
					>Exclude from transcoding</label
				>
				<p class="text-xs text-muted-foreground">
					{#if exclusion.status === LoadStatus.ERROR}
						<span class="text-destructive">{exclusion.error}</span>
					{:else}
						Keep this title's files as they are; queued transcodes are cancelled
					{/if}
				</p>
			</div>
			<Switch
				id="title-transcode-excluded"
				checked={exclusion.excluded}
				disabled={exclusion.status !== LoadStatus.READY || exclusion.saving}
				onCheckedChange={(checked) => exclusion.set(checked)}
			/>
		</div>

		<div class="grid gap-2">
			<span id="title-languages-label" class="text-sm font-medium">Subtitle languages</span>
			{#if languages.status === LoadStatus.LOADING}
				<Skeleton class="h-9 w-full sm:w-80" />
			{:else if languages.status === LoadStatus.ERROR}
				<div class="flex flex-wrap items-center gap-3">
					<p class="text-sm text-destructive">{languages.error}</p>
					<Button size="sm" variant="outline" onclick={() => languages.load()}>Retry</Button>
				</div>
			{:else}
				<Select.Root
					type="single"
					value={languages.source}
					onValueChange={(source) => languages.choose(source as LanguageSource)}
					disabled={languages.saving}
				>
					<Select.Trigger class="w-full sm:w-80" aria-labelledby="title-languages-label">
						{LANGUAGE_SOURCE_LABELS[languages.source]}
					</Select.Trigger>
					<Select.Content>
						{#each Object.values(LanguageSource) as source (source)}
							<Select.Item value={source} label={LANGUAGE_SOURCE_LABELS[source]} />
						{/each}
					</Select.Content>
				</Select.Root>
				{#if languages.source === LanguageSource.CUSTOM}
					<WantedLanguagesEditor list={languages.list} disabled={languages.saving} />
					{#if languages.unsaved}
						<div class="flex justify-end gap-2">
							<Button
								variant="ghost"
								disabled={languages.saving}
								onclick={() => languages.discard()}
							>
								Discard
							</Button>
							<Button disabled={languages.saving} onclick={() => languages.save()}>
								{languages.saving ? 'Saving…' : 'Save languages'}
							</Button>
						</div>
					{/if}
				{:else}
					<p class="text-xs text-muted-foreground">
						{languages.list.languages
							.map((entry) => languages.list.nameOf(entry.language))
							.join(', ')}
						· set in Settings → Subtitles
					</p>
				{/if}
			{/if}
		</div>

		<Separator />
		<div>
			<Button variant="destructive" onclick={() => page.askToRemove()}>Delete title…</Button>
		</div>
	</div>

	<AlertDialog.Root bind:open={page.confirmingRemove}>
		<AlertDialog.Content>
			<AlertDialog.Header>
				<AlertDialog.Title>Delete {page.title.name}?</AlertDialog.Title>
				<AlertDialog.Description>
					Magpie stops tracking this title. Files stay on disk unless you choose to delete them.
				</AlertDialog.Description>
			</AlertDialog.Header>
			<div class="flex items-center justify-between gap-4">
				<label for="delete-files" class="text-sm">Also delete its files from disk</label>
				<Switch id="delete-files" bind:checked={page.deleteFiles} />
			</div>
			<AlertDialog.Footer>
				<AlertDialog.Cancel disabled={page.removing}>Cancel</AlertDialog.Cancel>
				<AlertDialog.Action
					class="bg-destructive text-white hover:bg-destructive/90"
					disabled={page.removing}
					onclick={confirmRemove}
				>
					Delete
				</AlertDialog.Action>
			</AlertDialog.Footer>
		</AlertDialog.Content>
	</AlertDialog.Root>
{/if}
