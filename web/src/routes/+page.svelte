<script lang="ts">
	import AdminHome from '$lib/home/AdminHome.svelte';
	import RequesterHome from '$lib/home/RequesterHome.svelte';
	import { NAV_ITEMS } from '$lib/navigation/nav';
	import { getSession } from '$lib/session/session.svelte';

	const session = getSession();
</script>

<svelte:head>
	<title>{NAV_ITEMS.HOME.label} · Magpie</title>
</svelte:head>

{#if session.isAdmin}
	<AdminHome />
{:else if session.user}
	<RequesterHome userId={session.user.id} />
{/if}
