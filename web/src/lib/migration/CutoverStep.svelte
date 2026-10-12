<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Label } from '$lib/components/ui/label';
	import type { MigrationWizard } from './migration-wizard.svelte';

	let { wizard }: { wizard: MigrationWizard } = $props();

	const CHECKLIST = [
		'In Sonarr and Radarr, turn off RSS sync and automatic search, or stop them.',
		"Let their downloads in progress finish there. Magpie doesn't take over their torrents.",
		"Keep them stopped once Magpie has imported, so both don't grab the same titles."
	];
</script>

<div class="grid gap-4">
	<p class="text-sm text-muted-foreground">
		Magpie and the old apps must not both look for releases. Do this by hand before the commit.
	</p>
	<ol class="grid list-decimal gap-2 pl-5 text-sm">
		{#each CHECKLIST as item (item)}
			<li>{item}</li>
		{/each}
	</ol>
	<div class="flex items-start gap-3 rounded-lg border p-3">
		<Checkbox
			id="cutover-confirmed"
			checked={wizard.state?.cutoverConfirmed ?? false}
			disabled={wizard.busy}
			onCheckedChange={(on) => wizard.setCutover(on === true)}
		/>
		<Label for="cutover-confirmed" class="grid gap-1 font-normal">
			<span class="font-medium">I've done this</span>
			<span class="text-xs text-muted-foreground"
				>The Commit button stays off until this is ticked.</span
			>
		</Label>
	</div>
	<div class="flex justify-end">
		<Button onclick={() => wizard.next()}>Continue</Button>
	</div>
</div>
