<script lang="ts">
	import { getSession } from '$lib/session/session.svelte';
	import UsersSettings from '$lib/settings/users/UsersSettings.svelte';

	const session = getSession();
</script>

{#if session.user}
	<UsersSettings selfId={session.user.id} />
{/if}
