<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { PASSWORD_HINTS, PasswordForm, PasswordProblem } from './password-form.svelte';

	const form = new PasswordForm();
	const hint = $derived(
		form.next === '' || (form.problem === PasswordProblem.MISMATCH && form.confirmation === '')
			? undefined
			: form.problem && PASSWORD_HINTS[form.problem]
	);

	function submit(event: SubmitEvent) {
		event.preventDefault();
		form.submit();
	}
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Local admin password</Card.Title>
		<Card.Description>The fallback sign-in when Jellyfin is down.</Card.Description>
	</Card.Header>
	<Card.Content>
		<form class="grid gap-3" onsubmit={submit}>
			<div class="grid gap-1.5">
				<Label for="current-password">Current password</Label>
				<Input
					id="current-password"
					type="password"
					autocomplete="current-password"
					bind:value={form.current}
				/>
			</div>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div class="grid min-w-0 gap-1.5">
					<Label for="new-password">New password</Label>
					<Input
						id="new-password"
						type="password"
						autocomplete="new-password"
						bind:value={form.next}
					/>
				</div>
				<div class="grid min-w-0 gap-1.5">
					<Label for="confirm-password">Confirm new password</Label>
					<Input
						id="confirm-password"
						type="password"
						autocomplete="new-password"
						bind:value={form.confirmation}
					/>
				</div>
			</div>
			{#if hint}
				<p class="text-xs text-muted-foreground">{hint}</p>
			{/if}
			{#if form.error}
				<p class="text-sm text-destructive" role="alert">{form.error}</p>
			{/if}
			<div class="flex justify-end">
				<Button type="submit" size="sm" disabled={!form.canSubmit}>
					{form.submitting ? 'Changing…' : 'Change password'}
				</Button>
			</div>
		</form>
	</Card.Content>
</Card.Root>
