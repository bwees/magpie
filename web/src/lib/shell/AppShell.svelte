<script lang="ts">
	import type { Snippet } from 'svelte';
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { Toaster } from '$lib/components/ui/sonner';
	import AppSidebar from './AppSidebar.svelte';
	import BottomTabs from './BottomTabs.svelte';
	import TopBar from './TopBar.svelte';
	import { setShell, ShellState } from './shell-state.svelte';

	let { children }: { children: Snippet } = $props();

	const shell = setShell(new ShellState());
</script>

<Toaster position={shell.toastPosition} />

<Sidebar.Provider bind:open={shell.sidebarOpen}>
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
