<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import favicon from '$lib/assets/favicon.svg';
	import { Button } from '$lib/components/ui/button';
	import { Toaster } from '$lib/components/ui/sonner';
	import { IsMobile } from '$lib/hooks/is-mobile.svelte';
	import {
		isAuthRoute,
		SessionState,
		SessionStatus,
		setSession
	} from '$lib/session/session.svelte';
	import AppShell from '$lib/shell/AppShell.svelte';
	import RouteError from '$lib/shell/RouteError.svelte';
	import { ToastPosition } from '$lib/shell/shell-state.svelte';
	import './app.css';

	let { children }: { children: Snippet } = $props();

	const session = setSession(new SessionState());
	const isMobile = new IsMobile();
	const redirect = $derived(session.redirectFor(page.url.pathname));
	const bare = $derived(isAuthRoute(page.url.pathname));

	onMount(() => {
		session.load();
	});

	$effect(() => {
		if (redirect) goto(redirect, { replaceState: true });
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Magpie</title>
</svelte:head>

{#if session.status === SessionStatus.LOADING || redirect}
	<div class="flex min-h-svh items-center justify-center" role="status" aria-label="Loading">
		<LoaderCircle class="size-6 animate-spin text-muted-foreground" />
	</div>
{:else if session.status === SessionStatus.ERROR}
	<div class="flex min-h-svh flex-col items-center justify-center gap-3 p-4 text-center">
		<p class="text-sm text-muted-foreground">{session.error}</p>
		<Button variant="outline" onclick={() => session.load()}>Retry</Button>
	</div>
{:else if bare}
	<Toaster position={isMobile.current ? ToastPosition.PHONE : ToastPosition.DESKTOP} />
	{@render routeContent()}
{:else}
	<AppShell>
		{@render routeContent()}
	</AppShell>
{/if}

{#snippet routeContent()}
	<svelte:boundary onerror={(error) => console.error(error)}>
		{@render children()}
		{#snippet failed(error, reset)}
			<RouteError {error} {reset} />
		{/snippet}
	</svelte:boundary>
{/snippet}
