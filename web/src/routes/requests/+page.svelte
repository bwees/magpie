<script lang="ts">
	import RequestsScreen from '$lib/requests/RequestsScreen.svelte';
	import { getSession } from '$lib/session/session.svelte';

	const session = getSession();
</script>

<svelte:head>
	<title>Requests · Magpie</title>
</svelte:head>

{#if session.user}
	<RequestsScreen viewer={{ id: session.user.id, admin: session.isAdmin }} />
{/if}
