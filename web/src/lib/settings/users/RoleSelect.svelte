<script lang="ts">
	import * as Select from '$lib/components/ui/select';
	import { Role } from '$lib/session/session.svelte';
	import { cn } from '$lib/utils';
	import { ROLE_LABELS, type UserRow, type UsersManager } from './users-manager.svelte';

	let {
		row,
		manager,
		class: className
	}: { row: UserRow; manager: UsersManager; class?: string } = $props();

	const roles = Object.values(Role);
</script>

<Select.Root
	type="single"
	value={row.user.role}
	disabled={row.lockedReason !== undefined || row.saving}
	onValueChange={(role) => manager.setRole(row, role as Role)}
>
	<Select.Trigger class={cn('w-32', className)} aria-label={`Role of ${row.user.displayName}`}>
		{ROLE_LABELS[row.user.role as Role] ?? row.user.role}
	</Select.Trigger>
	<Select.Content>
		{#each roles as role (role)}
			<Select.Item value={role} label={ROLE_LABELS[role]} />
		{/each}
	</Select.Content>
</Select.Root>
