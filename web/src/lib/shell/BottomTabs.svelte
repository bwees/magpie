<script lang="ts">
	import { page } from '$app/state';
	import { BOTTOM_TABS, ownsPath } from '$lib/navigation/nav';
	import { cn } from '$lib/utils';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
</script>

<nav
	aria-label="Main"
	class="fixed inset-x-0 bottom-0 z-40 flex border-t bg-background/90 pb-[env(safe-area-inset-bottom)] backdrop-blur"
>
	{#each BOTTOM_TABS as tab (tab.route)}
		{@const active = ownsPath(tab, page.url.pathname)}
		{@const badge = shell.badgeFor(tab)}
		<a
			href={tab.route}
			aria-current={active ? 'page' : undefined}
			class={cn(
				'relative flex h-14 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 text-[10px]',
				active ? 'text-primary' : 'text-muted-foreground'
			)}
		>
			<tab.icon class="size-5" />
			<span class="truncate">{tab.label}</span>
			{#if badge}
				<span
					class="absolute top-1.5 left-1/2 ml-1.5 min-w-4 rounded-full bg-primary px-1 text-center text-[9px] leading-4 font-semibold text-primary-foreground"
				>
					{badge}
				</span>
			{/if}
		</a>
	{/each}
</nav>
