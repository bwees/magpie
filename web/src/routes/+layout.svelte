<script lang="ts">
	import type { Snippet } from 'svelte';
	import favicon from '$lib/assets/favicon.svg';
	import AppShell from '$lib/shell/AppShell.svelte';
	import './app.css';

	let { children }: { children: Snippet } = $props();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Magpie</title>
</svelte:head>

<AppShell>
	{@render children()}
</AppShell>
