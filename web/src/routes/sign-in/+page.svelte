<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import AuthFrame from '$lib/session/AuthFrame.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import { SignInForm } from '$lib/session/sign-in-form.svelte';

	const session = getSession();
	const form = new SignInForm();

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		const user = await form.submit();
		if (user) {
			session.signedIn(user);
			await goto('/', { replaceState: true });
		}
	}
</script>

<AuthFrame title="Sign in">
	<Card.Root>
		<Card.Header>
			<Card.Title>Local admin</Card.Title>
			<Card.Description>Sign in with the admin account created during setup.</Card.Description>
		</Card.Header>
		<Card.Content>
			<form class="grid gap-4" onsubmit={submit}>
				<div class="grid gap-1.5">
					<Label for="username">Username</Label>
					<Input id="username" autocomplete="username" bind:value={form.username} required />
				</div>
				<div class="grid gap-1.5">
					<Label for="password">Password</Label>
					<Input
						id="password"
						type="password"
						autocomplete="current-password"
						bind:value={form.password}
						required
					/>
				</div>
				{#if form.error}
					<p class="text-sm text-destructive" role="alert">{form.error}</p>
				{/if}
				<Button type="submit" disabled={!form.canSubmit}>
					{form.submitting ? 'Signing in…' : 'Sign in'}
				</Button>
			</form>
		</Card.Content>
	</Card.Root>
</AuthFrame>
