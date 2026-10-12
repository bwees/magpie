<script lang="ts">
	import RequestsView from '$lib/requests/RequestsView.svelte';
	import { getSession } from '$lib/session/session.svelte';

	const session = getSession();
</script>

<svelte:head>
	<title>Requests · Magpie</title>
</svelte:head>

{#if session.user}
	<RequestsView viewer={{ id: session.user.id, admin: session.isAdmin }} />
{/if}
