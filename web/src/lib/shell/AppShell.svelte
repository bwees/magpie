<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { Toaster } from '$lib/components/ui/sonner';
	import { LiveClient, setLive } from '$lib/live/live-client.svelte';
	import { liveUrl } from '$lib/live/protocol';
	import { apiBaseUrl } from '$lib/sdk';
	import { getSession } from '$lib/session/session.svelte';
	import AppSidebar from './AppSidebar.svelte';
	import BottomTabs from './BottomTabs.svelte';
	import TopBar from './TopBar.svelte';
	import { setShell, ShellState } from './shell-state.svelte';

	let { children }: { children: Snippet } = $props();

	const shell = setShell(new ShellState());
	const live = setLive(new LiveClient({ url: liveUrl(apiBaseUrl, location.href) }));

	const session = getSession();

	onMount(() => {
		live.connect();
		const stopBadges = shell.watchBadges(live, session.isAdmin);
		return () => {
			stopBadges();
			live.close();
		};
	});
</script>

<Toaster position={shell.toastPosition} />

<Sidebar.Provider
	bind:open={shell.sidebarOpen}
	class="pointer-coarse:[--sidebar-width-icon:3.5rem]!"
>
	<AppSidebar />

	<Sidebar.Inset class="min-w-0">
		<TopBar />
		<main class="flex-1 p-4 pb-[calc(--spacing(18)+env(safe-area-inset-bottom))] md:p-6 md:pb-6">
			{@render children()}
		</main>
	</Sidebar.Inset>

	{#if shell.isMobile}
		<BottomTabs />
	{/if}
</Sidebar.Provider>
