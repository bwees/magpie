<script lang="ts">
	import { goto } from '$app/navigation';
	import Construction from '@lucide/svelte/icons/construction';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import ConnectionFields from '$lib/connections/ConnectionFields.svelte';
	import HealthBadge from '$lib/connections/HealthBadge.svelte';
	import { LoadStatus } from '$lib/load-status';
	import AuthFrame from '$lib/session/AuthFrame.svelte';
	import { getSession } from '$lib/session/session.svelte';
	import { SetupWizard } from '$lib/setup/setup-wizard.svelte';
	import StepList from '$lib/setup/StepList.svelte';
	import { StepKind, WIZARD_STEPS } from '$lib/setup/steps';

	const session = getSession();
	const wizard = new SetupWizard();

	const step = $derived(wizard.current);
	const form = $derived(step.service && wizard.connections.forms[step.service]);
	const health = $derived(step.service && wizard.connections.health[step.service]);

	async function createAdmin(event: SubmitEvent) {
		event.preventDefault();
		const user = await wizard.createAdmin();
		if (user) session.signedIn(user, { startWizard: true });
	}

	$effect(() => {
		if (wizard.finished) {
			session.finishWizard();
			goto('/', { replaceState: true });
		}
	});
</script>

<AuthFrame title="Set up Magpie" wide>
	<div class="grid gap-6 md:grid-cols-[13rem_1fr]">
		<aside class="order-last md:order-none">
			<StepList {wizard} />
		</aside>

		<Card.Root class="min-w-0">
			<Card.Header>
				<p class="text-xs text-muted-foreground">
					Step {wizard.index + 1} of {WIZARD_STEPS.length}
				</p>
				<Card.Title class="flex flex-wrap items-center gap-2">
					{step.label}
					{#if health}
						<HealthBadge {health} />
					{/if}
				</Card.Title>
				<Card.Description>{step.description}</Card.Description>
			</Card.Header>

			{#if step.kind === StepKind.ACCOUNT}
				<Card.Content>
					<form id="admin-form" class="grid gap-3" onsubmit={createAdmin}>
						<div class="grid gap-1.5">
							<Label for="admin-username">Username</Label>
							<Input
								id="admin-username"
								autocomplete="username"
								bind:value={wizard.admin.username}
							/>
						</div>
						<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
							<div class="grid min-w-0 gap-1.5">
								<Label for="admin-password">Password</Label>
								<Input
									id="admin-password"
									type="password"
									autocomplete="new-password"
									bind:value={wizard.admin.password}
								/>
							</div>
							<div class="grid min-w-0 gap-1.5">
								<Label for="admin-confirm">Confirm password</Label>
								<Input
									id="admin-confirm"
									type="password"
									autocomplete="new-password"
									bind:value={wizard.admin.confirmation}
								/>
							</div>
						</div>
						{#if wizard.admin.hint}
							<p class="text-xs text-muted-foreground">{wizard.admin.hint}</p>
						{/if}
						{#if wizard.admin.error}
							<p class="text-sm text-destructive" role="alert">{wizard.admin.error}</p>
						{/if}
					</form>
				</Card.Content>
				<Card.Footer class="justify-end">
					<Button type="submit" form="admin-form" disabled={!wizard.admin.canSubmit}>
						{wizard.admin.submitting ? 'Creating…' : 'Create admin'}
					</Button>
				</Card.Footer>
			{:else if step.kind === StepKind.CONNECTION && form && step.service}
				{@const service = step.service}
				<Card.Content>
					{#if wizard.connections.status === LoadStatus.LOADING}
						<Skeleton class="h-16 w-full" />
					{:else if wizard.connections.status === LoadStatus.ERROR}
						<div class="flex flex-wrap items-center gap-3">
							<p class="text-sm text-destructive">{wizard.connections.error}</p>
							<Button size="sm" variant="outline" onclick={() => wizard.connections.load()}>
								Retry
							</Button>
						</div>
					{:else}
						<ConnectionFields {form} disabled={form.busy} />
					{/if}
				</Card.Content>
				<Card.Footer class="flex-wrap justify-end gap-2">
					<Button variant="ghost" disabled={form.busy} onclick={() => wizard.skip()}>Skip</Button>
					<Button
						variant="outline"
						disabled={form.busy || wizard.connections.status !== LoadStatus.READY}
						onclick={() => wizard.connections.test(service)}
					>
						{form.testing ? 'Testing…' : 'Test'}
					</Button>
					<Button disabled={form.busy} onclick={() => wizard.complete()}>
						{wizard.isLast ? 'Finish' : 'Continue'}
					</Button>
				</Card.Footer>
			{:else}
				<Card.Content>
					<div
						class="flex flex-col items-center gap-2 rounded-lg border border-dashed p-6 text-center"
					>
						<Construction class="size-5 text-muted-foreground" />
						<p class="text-sm text-muted-foreground">
							Coming soon. Skip it for now and set it up later in Settings.
						</p>
					</div>
				</Card.Content>
				<Card.Footer class="justify-end">
					<Button onclick={() => wizard.skip()}>{wizard.isLast ? 'Finish' : 'Skip'}</Button>
				</Card.Footer>
			{/if}
		</Card.Root>
	</div>
</AuthFrame>
