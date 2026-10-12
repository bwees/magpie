<script lang="ts">
	import BackupsCard from '$lib/backups/BackupsCard.svelte';
	import LocalPasswordCard from '$lib/session/LocalPasswordCard.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import SchedulesCard from '$lib/settings/schedules/SchedulesCard.svelte';

	const session = getSession();
</script>

<div class="grid gap-4">
	{#if session.user?.localAdmin}
		<LocalPasswordCard />
	{/if}
	<BackupsCard />
	<SchedulesCard />
</div>
