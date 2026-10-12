<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import type { SearchRequests } from './search-requests.svelte';

	let { requests, actionLabel }: { requests: SearchRequests; actionLabel: string } = $props();

	const picker = $derived(requests.picker);
</script>

<Dialog.Root
	open={picker !== undefined}
	onOpenChange={(open) => {
		if (!open) requests.closePicker();
	}}
>
	<Dialog.Content class="sm:max-w-md">
		{#if picker}
			<Dialog.Header>
				<Dialog.Title>{actionLabel} {picker.item.name}</Dialog.Title>
				<Dialog.Description>The whole series follows new seasons too.</Dialog.Description>
			</Dialog.Header>
			<div class="flex items-center justify-between gap-3">
				<Label for="whole-series">Whole series</Label>
				<Switch
					id="whole-series"
					checked={picker.wholeSeries}
					onCheckedChange={(on) => (picker.wholeSeries = on)}
				/>
			</div>
			{#if picker.status === LoadStatus.LOADING}
				<Skeleton class="h-32 w-full" />
			{:else if picker.status === LoadStatus.ERROR}
				<div class="flex flex-wrap items-center gap-2" role="alert">
					<span class="text-sm text-destructive">{picker.error}</span>
					<Button size="sm" variant="outline" onclick={() => picker.load()}>Retry</Button>
				</div>
			{:else}
				<ul class="grid max-h-72 gap-1 overflow-y-auto" aria-label="Seasons">
					{#each picker.seasons as season (season.number)}
						{@const id = `season-${season.number}`}
						<li class="flex items-center gap-3 rounded-md px-1 py-1.5">
							<Checkbox
								{id}
								checked={season.monitored ||
									(!picker.wholeSeries && picker.chosen.has(season.number))}
								disabled={season.monitored}
								onCheckedChange={(on) => picker.toggle(season.number, on === true)}
							/>
							<Label for={id} class="flex-1 font-normal">{picker.seasonLabel(season)}</Label>
							<span class="text-xs text-muted-foreground">
								{season.monitored ? 'In library' : `${season.episodeCount} episodes`}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
			<Dialog.Footer>
				<Button variant="outline" onclick={() => requests.closePicker()}>Cancel</Button>
				<Button
					disabled={!picker.canSubmit || requests.submitting}
					onclick={() => requests.submitPicker()}
				>
					{actionLabel}
				</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
