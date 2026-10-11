<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Route } from '$lib/navigation/nav';
	import type { MeResponse } from '$lib/sdk';
	import AuthFrame from '$lib/session/AuthFrame.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import {
		QuickConnectStep,
		SignInErrorKind,
		SignInMode,
		SignInPage
	} from '$lib/session/sign-in-page.svelte';

	const session = getSession();
	const signIn = new SignInPage(async (user: MeResponse) => {
		session.signedIn(user);
		await goto(Route.HOME, { replaceState: true });
	});

	const jellyfinDown = $derived(
		signIn.offersJellyfin && !signIn.methods?.jellyfinReachable && signIn.mode !== SignInMode.LOCAL
	);

	onMount(() => {
		signIn.load();
	});
	onDestroy(() => signIn.stop());

	function submit(event: SubmitEvent) {
		event.preventDefault();
		signIn.submit();
	}
</script>

{#snippet errorMessage()}
	{#if signIn.error}
		<div class="grid gap-2 text-sm" role="alert">
			<p class="text-destructive">{signIn.error.message}</p>
			{#if signIn.error.kind === SignInErrorKind.JELLYFIN_UNAVAILABLE}
				<p class="text-muted-foreground">
					The local admin can still sign in.
					<button
						type="button"
						class="font-medium text-foreground underline underline-offset-4"
						onclick={() => signIn.useLocalAdmin()}
					>
						Use local admin
					</button>
				</p>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet credentials(usernameLabel: string)}
	<form class="grid gap-4" onsubmit={submit}>
		<div class="grid gap-1.5">
			<Label for="username">{usernameLabel}</Label>
			<Input id="username" autocomplete="username" bind:value={signIn.username} required />
		</div>
		<div class="grid gap-1.5">
			<Label for="password">Password</Label>
			<Input
				id="password"
				type="password"
				autocomplete="current-password"
				bind:value={signIn.password}
				required
			/>
		</div>
		{@render errorMessage()}
		<Button type="submit" disabled={!signIn.canSubmit}>
			{signIn.submitting ? 'Signing in…' : 'Sign in'}
		</Button>
	</form>
{/snippet}

<AuthFrame title="Sign in">
	{#if !signIn.methods}
		<div class="flex justify-center py-12" role="status" aria-label="Loading">
			<LoaderCircle class="size-6 animate-spin text-muted-foreground" />
		</div>
	{:else}
		{#if jellyfinDown}
			<Alert.Root>
				<CircleAlert />
				<Alert.Title>Jellyfin isn't responding</Alert.Title>
				<Alert.Description
					>Jellyfin sign-in may fail. The local admin can still sign in.</Alert.Description
				>
			</Alert.Root>
		{/if}

		<Card.Root>
			{#if signIn.mode === SignInMode.JELLYFIN}
				<Card.Header>
					<Card.Title>Sign in with Jellyfin</Card.Title>
					<Card.Description>Use your Jellyfin username and password.</Card.Description>
				</Card.Header>
				<Card.Content class="grid gap-4">
					{@render credentials('Jellyfin username')}
					{#if signIn.methods.quickConnect}
						<div class="flex items-center gap-3 text-xs text-muted-foreground">
							<Separator class="flex-1" />
							or
							<Separator class="flex-1" />
						</div>
						<Button variant="outline" onclick={() => signIn.startQuickConnect()}>
							Use Quick Connect
						</Button>
					{/if}
				</Card.Content>
			{:else if signIn.mode === SignInMode.QUICK_CONNECT}
				<Card.Header>
					<Card.Title>Quick Connect</Card.Title>
					<Card.Description>
						In a Jellyfin app where you're signed in, open Quick Connect and enter this code.
					</Card.Description>
				</Card.Header>
				<Card.Content class="grid gap-4">
					{#if signIn.quickConnectStep === QuickConnectStep.STARTING}
						<div class="flex justify-center py-4" role="status" aria-label="Getting a code">
							<LoaderCircle class="size-6 animate-spin text-muted-foreground" />
						</div>
					{:else if signIn.quickConnectStep === QuickConnectStep.WAITING}
						<p
							class="text-center font-mono text-4xl font-semibold tracking-[0.3em]"
							aria-label="Quick Connect code"
						>
							{signIn.quickConnectCode}
						</p>
						<p
							class="flex items-center justify-center gap-2 text-sm text-muted-foreground"
							role="status"
						>
							<LoaderCircle class="size-4 animate-spin" />
							Waiting for approval…
						</p>
					{:else if signIn.quickConnectStep === QuickConnectStep.EXPIRED}
						<p class="text-sm text-muted-foreground" role="alert">The code expired.</p>
						<Button onclick={() => signIn.startQuickConnect()}>Get a new code</Button>
					{:else}
						{@render errorMessage()}
						<Button onclick={() => signIn.startQuickConnect()}>Try again</Button>
					{/if}
					<Button variant="ghost" onclick={() => signIn.useJellyfin()}>
						Use username and password
					</Button>
				</Card.Content>
			{:else}
				<Card.Header>
					<Card.Title>Local admin</Card.Title>
					<Card.Description>Sign in with the admin account created during setup.</Card.Description>
				</Card.Header>
				<Card.Content>
					{@render credentials('Username')}
				</Card.Content>
			{/if}
		</Card.Root>

		<p class="text-center text-sm text-muted-foreground">
			{#if signIn.mode !== SignInMode.LOCAL}
				<button
					type="button"
					class="underline underline-offset-4 hover:text-foreground"
					onclick={() => signIn.useLocalAdmin()}
				>
					Local admin
				</button>
			{:else if signIn.offersJellyfin}
				<button
					type="button"
					class="underline underline-offset-4 hover:text-foreground"
					onclick={() => signIn.useJellyfin()}
				>
					Sign in with Jellyfin
				</button>
			{/if}
		</p>
	{/if}
</AuthFrame>
