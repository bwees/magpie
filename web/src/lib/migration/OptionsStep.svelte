<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const OPTIONS = [
		{
			id: 'renameFolders',
			label: 'Rename existing folders',
			note: 'Moves each file and its subtitles to Title (Year) [tmdbid-N]. Off, files stay where they are.'
		},
		{
			id: 'importAliases',
			label: "Import Sonarr's alternate titles",
			note: 'Adds them to the names Magpie matches releases against.'
		}
	] as const;
</script>

<div class="grid gap-4">
	<ul class="grid gap-3">
		{#each OPTIONS as option (option.id)}
			<li class="flex items-start justify-between gap-4 rounded-lg border p-3">
				<div class="grid gap-0.5">
					<label for="option-{option.id}" class="text-sm font-medium">{option.label}</label>
					<p class="text-xs text-muted-foreground">{option.note}</p>
				</div>
				<Switch
					id="option-{option.id}"
					checked={wizard.state?.options[option.id] ?? false}
					disabled={wizard.busy}
					onCheckedChange={(on) => wizard.setOption(option.id, on)}
				/>
			</li>
		{/each}
	</ul>
	<div class="flex justify-end">
		<Button onclick={() => wizard.next()}>Continue</Button>
	</div>
</div>
