<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import type { ImportEpisodeResponse } from '$lib/sdk';
	import { formatSize } from '$lib/search/format';
	import { baseName, episodeLabel } from './library';
	import type { ImportsManager } from './imports-manager.svelte';

	let { manager }: { manager: ImportsManager } = $props();

	const imp = $derived(manager.mapping);
	const MAX_EPISODES_SHOWN = 3;

	function episodeName(e: ImportEpisodeResponse): string {
		const label = episodeLabel([{ episode: e.id, season: e.season, number: e.number }]);
		return e.name ? `${label} · ${e.name}` : label;
	}

	function chosenLabel(ids: string[]): string {
		if (ids.length === 0) {
			return 'Pick episodes';
		}
		const names = (imp?.episodes ?? [])
			.filter((e) => ids.includes(e.id))
			.map((e) => episodeLabel([{ episode: e.id, season: e.season, number: e.number }]));
		return names.length > MAX_EPISODES_SHOWN
			? `${names.slice(0, MAX_EPISODES_SHOWN).join(', ')} +${names.length - MAX_EPISODES_SHOWN}`
			: names.join(', ');
	}

	function fileOf(path: string) {
		return imp?.files.find((f) => f.path === path);
	}
</script>

<Dialog.Root
	open={imp !== undefined}
	onOpenChange={(open) => {
		if (!open) manager.closeMapping();
	}}
>
	<Dialog.Content class="sm:max-w-2xl">
		{#if imp}
			<Dialog.Header>
				<Dialog.Title>Map files · {imp.titleName}</Dialog.Title>
				<Dialog.Description>
					{manager.mappingMovie
						? 'Pick the video that is the movie.'
						: 'Pick the episodes each video holds. Videos you leave out aren’t imported.'}
				</Dialog.Description>
			</Dialog.Header>
			<ul class="grid max-h-[60vh] gap-2 overflow-y-auto" aria-label="Videos">
				{#each manager.choices as choice, i (choice.path)}
					{@const file = fileOf(choice.path)}
					{@const id = `map-video-${i}`}
					<li class="grid gap-2 rounded-lg border p-3">
						<div class="flex items-start gap-3">
							<Checkbox
								{id}
								checked={choice.included}
								onCheckedChange={(on) => manager.toggleVideo(choice.path, on === true)}
							/>
							<Label for={id} class="grid min-w-0 flex-1 gap-1 font-normal">
								<span class="font-mono text-xs break-all">{baseName(choice.path)}</span>
								<span class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
									{#if file}{formatSize(file.size)}{/if}
									{#if file?.sample}<Badge variant="secondary">Sample</Badge>{/if}
								</span>
							</Label>
						</div>
						{#if !manager.mappingMovie}
							<Select.Root
								type="multiple"
								value={choice.episodes}
								onValueChange={(value) => (choice.episodes = value)}
								disabled={!choice.included}
							>
								<Select.Trigger class="w-full" aria-label="Episodes of {baseName(choice.path)}">
									{chosenLabel(choice.episodes)}
								</Select.Trigger>
								<Select.Content class="max-h-72">
									{#each imp.episodes as episode (episode.id)}
										<Select.Item value={episode.id} label={episodeName(episode)} />
									{/each}
								</Select.Content>
							</Select.Root>
						{/if}
					</li>
				{/each}
			</ul>
			{#if manager.mappingError}
				<p class="text-sm text-destructive" role="alert">{manager.mappingError}</p>
			{/if}
			<Dialog.Footer>
				<Button variant="outline" onclick={() => manager.closeMapping()}>Cancel</Button>
				<Button disabled={manager.submitting} onclick={() => manager.submitMapping()}>Import</Button
				>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
