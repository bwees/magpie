<script lang="ts">
	import { page } from '$app/state';
	import Bell from '@lucide/svelte/icons/bell';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import Search from '@lucide/svelte/icons/search';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import AvatarMenu from './AvatarMenu.svelte';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
	const current = $derived(shell.pageFor(page.url.pathname));
</script>

<header class="sticky top-0 z-30 border-b bg-background/80 backdrop-blur">
	<div class="flex h-14 items-center gap-3 px-4 md:gap-4 md:px-6">
		{#if current}
			<h1 class="truncate text-sm font-medium">{current.label}</h1>
		{:else}
			<button
				type="button"
				class="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
				onclick={() => history.back()}
			>
				<ChevronLeft class="size-4" />
				Back
			</button>
		{/if}

		<div class="ml-auto flex items-center gap-1 md:gap-3">
			{#if shell.isMobile}
				<Button variant="ghost" size="icon" class="rounded-full" aria-label="Search" disabled>
					<Search class="size-5" />
				</Button>
			{:else}
				<div class="relative w-56 lg:w-80">
					<Search
						class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						type="search"
						class="pl-8"
						placeholder="Search titles"
						aria-label="Search titles"
						disabled
					/>
				</div>
			{/if}

			<Button variant="ghost" size="icon" class="relative rounded-full" aria-label="Notifications">
				<Bell class="size-4" />
				{#if shell.hasUnreadNotifications}
					<span class="absolute top-2 right-2 size-2 rounded-full bg-primary"></span>
				{/if}
			</Button>

			<AvatarMenu />
		</div>
	</div>
</header>
