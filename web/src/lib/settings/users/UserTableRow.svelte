<script lang="ts">
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import RoleSelect from './RoleSelect.svelte';
	import UserIdentity from './UserIdentity.svelte';
	import {
		PERMISSION_TOGGLES,
		USER_TABLE_COLUMNS,
		type UserRow,
		type UsersManager
	} from './users-manager.svelte';

	let { row, manager }: { row: UserRow; manager: UsersManager } = $props();

	const locked = $derived(row.lockedReason !== undefined || row.saving);
	const note = $derived(row.error ?? row.lockedReason);
</script>

<Table.Row class={note ? 'border-b-0' : undefined}>
	<Table.Cell class="max-w-56">
		<div class="flex items-center gap-2">
			<UserIdentity {row} />
			{#if row.saving}
				<LoaderCircle class="size-3.5 shrink-0 animate-spin text-muted-foreground" />
			{/if}
		</div>
	</Table.Cell>
	<Table.Cell>
		<RoleSelect {row} {manager} />
	</Table.Cell>
	{#each PERMISSION_TOGGLES as toggle (toggle.key)}
		<Table.Cell class="text-center">
			<Switch
				checked={row.user.permissions[toggle.key]}
				disabled={locked || row.isAdmin}
				aria-label={`${toggle.label} for ${row.user.displayName}`}
				onCheckedChange={(value) => manager.setPermission(row, toggle.key, value)}
			/>
		</Table.Cell>
	{/each}
	<Table.Cell class="text-center">
		<Switch
			checked={!row.user.disabled}
			disabled={locked}
			aria-label={`${row.user.displayName} enabled`}
			onCheckedChange={(value) => manager.setEnabled(row, value)}
		/>
	</Table.Cell>
</Table.Row>
{#if note}
	<Table.Row class="hover:bg-transparent">
		<Table.Cell colspan={USER_TABLE_COLUMNS.length} class="pt-0 text-xs">
			<span
				class={row.error ? 'text-destructive' : 'text-muted-foreground'}
				role={row.error ? 'alert' : undefined}
			>
				{note}
			</span>
		</Table.Cell>
	</Table.Row>
{/if}
