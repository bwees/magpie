<script lang="ts">
	import { page } from '$app/state';
	import { bottomTabsFor, isActionTab, ownsPath, type BottomTab } from '$lib/navigation/nav';
	import { getSession } from '$lib/session/session.svelte';
	import { cn } from '$lib/utils';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
	const session = getSession();
	const tabs = $derived(bottomTabsFor(session.navRole));

	const tabClass =
		'relative flex h-14 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[10px]';
</script>

{#snippet tabContent(tab: BottomTab, badge: number)}
	<tab.icon class="size-5" />
	<span class="truncate">{tab.label}</span>
	{#if badge}
		<span
			class="absolute top-1.5 left-1/2 ml-1.5 min-w-4 rounded-full bg-primary px-1 text-center text-[9px] leading-4 font-semibold text-primary-foreground"
		>
			{badge}
		</span>
	{/if}
{/snippet}

<nav
	aria-label="Main"
	class="fixed inset-x-0 bottom-0 z-40 flex border-t bg-background/90 pb-[env(safe-area-inset-bottom)] backdrop-blur"
>
	{#each tabs as tab (tab.label)}
		{#if isActionTab(tab)}
			<button
				type="button"
				class={cn(tabClass, 'text-muted-foreground')}
				onclick={() => shell.run(tab.action)}
			>
				{@render tabContent(tab, 0)}
			</button>
		{:else}
			{@const active = ownsPath(tab, page.url.pathname)}
			<a
				href={tab.route}
				aria-current={active ? 'page' : undefined}
				class={cn(tabClass, active ? 'text-primary' : 'text-muted-foreground')}
			>
				{@render tabContent(tab, shell.badgeFor(tab))}
			</a>
		{/if}
	{/each}
</nav>
