<script lang="ts">
	import * as Select from '$lib/components/ui/select';
	import { SeedingMode } from '$lib/downloads/downloads';
	import { SEEDING_MODE_HINTS, SEEDING_MODE_LABELS } from './download-options';
	import type { DownloadOptionsManager } from './download-options-manager.svelte';

	let { manager, id }: { manager: DownloadOptionsManager; id: string } = $props();

	const mode = $derived(
		(manager.saved?.seedingMode ?? SeedingMode.SEED_UNTIL_GOALS) as SeedingMode
	);
</script>

<div class="grid gap-1.5">
	<Select.Root
		type="single"
		value={mode}
		disabled={manager.saving}
		onValueChange={(value) => manager.setSeedingMode(value as SeedingMode)}
	>
		<Select.Trigger {id} class="w-full sm:w-56" aria-label="After download">
			{SEEDING_MODE_LABELS[mode]}
		</Select.Trigger>
		<Select.Content>
			{#each Object.values(SeedingMode) as option (option)}
				<Select.Item value={option} label={SEEDING_MODE_LABELS[option]} />
			{/each}
		</Select.Content>
	</Select.Root>
	<p class="text-xs text-muted-foreground">{SEEDING_MODE_HINTS[mode]}</p>
</div>
