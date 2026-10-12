<script lang="ts">
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { getLive } from '$lib/live/live-client.svelte';
	import MigrationWizard from '$lib/migration/MigrationWizard.svelte';
	import { settingsPath, SettingsSection } from '$lib/settings/sections';

	const live = getLive();
</script>

<svelte:head>
	<title>Import from Sonarr/Radarr · Settings · Magpie</title>
</svelte:head>

<div class="grid gap-3">
	<Button variant="ghost" size="sm" class="w-fit" href={settingsPath(SettingsSection.GENERAL)}>
		<ArrowLeft class="size-4" /> General
	</Button>
	<Card.Root class="min-w-0">
		<Card.Header>
			<Card.Title>Import from Sonarr/Radarr</Card.Title>
			<Card.Description>
				A one-time import. Nothing is written until you commit, and a failed commit resumes where it
				stopped.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<MigrationWizard {live} />
		</Card.Content>
	</Card.Root>
</div>
