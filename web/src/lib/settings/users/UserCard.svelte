<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import RoleSelect from './RoleSelect.svelte';
	import UserIdentity from './UserIdentity.svelte';
	import { PERMISSION_TOGGLES, type UserRow, type UsersManager } from './users-manager.svelte';

	let { row, manager }: { row: UserRow; manager: UsersManager } = $props();

	const locked = $derived(row.lockedReason !== undefined || row.saving);
	const idPrefix = $derived(`user-${row.user.id}`);
</script>

<Card.Root size="sm">
	<Card.Content class="flex min-w-0 flex-col gap-3">
		<div class="flex items-start justify-between gap-3">
			<UserIdentity {row} />
			<RoleSelect {row} {manager} class="shrink-0" />
		</div>
		<div class="flex flex-col gap-2.5">
			{#each PERMISSION_TOGGLES as toggle (toggle.key)}
				<div class="flex items-center justify-between gap-3">
					<Label for={`${idPrefix}-${toggle.key}`} class="font-normal">{toggle.label}</Label>
					<Switch
						id={`${idPrefix}-${toggle.key}`}
						checked={row.user.permissions[toggle.key]}
						disabled={locked || row.isAdmin}
						onCheckedChange={(value) => manager.setPermission(row, toggle.key, value)}
					/>
				</div>
			{/each}
			<div class="flex items-center justify-between gap-3">
				<Label for={`${idPrefix}-enabled`} class="font-normal">Enabled</Label>
				<Switch
					id={`${idPrefix}-enabled`}
					checked={!row.user.disabled}
					disabled={locked}
					onCheckedChange={(value) => manager.setEnabled(row, value)}
				/>
			</div>
		</div>
		{#if row.error}
			<p class="text-xs text-destructive" role="alert">{row.error}</p>
		{:else if row.lockedReason}
			<p class="text-xs text-muted-foreground">{row.lockedReason}</p>
		{/if}
	</Card.Content>
</Card.Root>
