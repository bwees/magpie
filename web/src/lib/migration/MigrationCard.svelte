<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { MIGRATION_PATH } from './migration';
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Import from Sonarr/Radarr</Card.Title>
		<Card.Description>
			Bring over titles, files, quality profiles, custom formats, the blocklist and history once,
			with a dry run first.
		</Card.Description>
		<Card.Action>
			<Button size="sm" variant="outline" href={MIGRATION_PATH}>Open</Button>
		</Card.Action>
	</Card.Header>
</Card.Root>
