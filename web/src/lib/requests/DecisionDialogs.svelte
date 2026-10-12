<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Dialog from '$lib/components/ui/dialog';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import ProfileSelect from '$lib/settings/quality/ProfileSelect.svelte';
	import type { RequestsPage } from './requests-page.svelte';

	let { page }: { page: RequestsPage } = $props();

	const approveBusy = $derived(page.approving ? page.busy.has(page.approving.id) : false);
	const declineBusy = $derived(page.declining ? page.busy.has(page.declining.id) : false);
</script>

<Dialog.Root
	open={page.approving !== undefined}
	onOpenChange={(open) => {
		if (!open) page.approving = undefined;
	}}
>
	<Dialog.Content class="sm:max-w-md">
		{#if page.approving}
			<Dialog.Header>
				<Dialog.Title>Approve {page.titleOf(page.approving)}?</Dialog.Title>
				<Dialog.Description>
					Magpie adds it to the library and starts searching with this quality profile.
				</Dialog.Description>
			</Dialog.Header>
			<div class="grid gap-2">
				<Label for="approve-profile">Quality profile</Label>
				<ProfileSelect
					id="approve-profile"
					value={page.approveProfile}
					onChange={(profileId) => (page.approveProfile = profileId)}
				/>
			</div>
			<Dialog.Footer>
				<Button variant="outline" onclick={() => (page.approving = undefined)}>Cancel</Button>
				<Button disabled={approveBusy} onclick={() => page.approve()}>Approve</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root
	open={page.declining !== undefined}
	onOpenChange={(open) => {
		if (!open) page.declining = undefined;
	}}
>
	<Dialog.Content class="sm:max-w-md">
		{#if page.declining}
			<Dialog.Header>
				<Dialog.Title>Decline {page.titleOf(page.declining)}?</Dialog.Title>
				<Dialog.Description>
					{page.declining.requester.displayName || 'The requester'} sees the reason, if you give one.
				</Dialog.Description>
			</Dialog.Header>
			<div class="grid gap-2">
				<Label for="decline-reason">Reason (optional)</Label>
				<Input
					id="decline-reason"
					bind:value={page.declineReason}
					placeholder="Already on another server"
				/>
			</div>
			<Dialog.Footer>
				<Button variant="outline" onclick={() => (page.declining = undefined)}>Cancel</Button>
				<Button variant="destructive" disabled={declineBusy} onclick={() => page.decline()}>
					Decline
				</Button>
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
