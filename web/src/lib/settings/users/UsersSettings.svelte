<script lang="ts">
	import { onMount } from 'svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import * as Table from '$lib/components/ui/table';
	import { LoadStatus } from '$lib/load-status';
	import UserCard from './UserCard.svelte';
	import UserTableRow from './UserTableRow.svelte';
	import { PERMISSION_TOGGLES, USER_TABLE_COLUMNS, UsersManager } from './users-manager.svelte';

	let { selfId }: { selfId: string } = $props();

	const SKELETON_ROWS = ['first', 'second', 'third'];

	// svelte-ignore state_referenced_locally
	const manager = new UsersManager(selfId);

	onMount(() => {
		manager.load();
	});
</script>

<div class="grid gap-4">
	{#if manager.status === LoadStatus.ERROR}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Could not load users</Alert.Title>
			<Alert.Description>
				<p>{manager.loadError}</p>
				<Button variant="outline" size="sm" class="mt-2" onclick={() => manager.load()}>
					Retry
				</Button>
			</Alert.Description>
		</Alert.Root>
	{:else}
		<Card.Root>
			<Card.Header>
				<Card.Title>Users</Card.Title>
				<Card.Description>
					Anyone with a Jellyfin account can sign in and starts as a requester. Admins see and do
					everything.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				{#if manager.status === LoadStatus.LOADING}
					<div class="grid gap-3" role="status" aria-label="Loading users">
						{#each SKELETON_ROWS as key (key)}
							<Skeleton class="h-10 w-full" />
						{/each}
					</div>
				{:else}
					<div class="hidden md:block">
						<Table.Root>
							<Table.Header>
								<Table.Row>
									{#each USER_TABLE_COLUMNS as column (column.label)}
										<Table.Head
											class={column.isSwitch
												? 'w-20 text-center text-xs leading-tight whitespace-normal'
												: undefined}
										>
											{column.label}
										</Table.Head>
									{/each}
								</Table.Row>
							</Table.Header>
							<Table.Body>
								{#each manager.rows as row (row.user.id)}
									<UserTableRow {row} {manager} />
								{/each}
							</Table.Body>
						</Table.Root>
					</div>
					<div class="flex min-w-0 flex-col gap-3 md:hidden">
						{#each manager.rows as row (row.user.id)}
							<UserCard {row} {manager} />
						{/each}
					</div>
				{/if}
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Defaults for new users</Card.Title>
				<Card.Description>
					Jellyfin users get these permissions the first time they sign in.
				</Card.Description>
			</Card.Header>
			<Card.Content class="grid gap-3">
				{#if manager.defaults}
					{#each PERMISSION_TOGGLES as toggle (toggle.key)}
						<div class="flex items-center justify-between gap-3">
							<Label for={`default-${toggle.key}`} class="font-normal">{toggle.label}</Label>
							<Switch
								id={`default-${toggle.key}`}
								checked={manager.defaults[toggle.key]}
								onCheckedChange={(value) => manager.setDefault(toggle.key, value)}
							/>
						</div>
					{/each}
					{#if manager.defaultsError}
						<p class="text-xs text-destructive" role="alert">{manager.defaultsError}</p>
					{/if}
				{:else}
					<Skeleton class="h-24 w-full" />
				{/if}
			</Card.Content>
		</Card.Root>

		<Card.Root>
			<Card.Header>
				<Card.Title>Sign-in</Card.Title>
			</Card.Header>
			<Card.Content class="grid gap-2">
				<div class="flex items-center justify-between gap-3">
					<div class="grid gap-0.5">
						<Label for="allow-jellyfin">Allow Jellyfin sign-in</Label>
						<p class="text-xs text-muted-foreground">The local admin can always sign in.</p>
					</div>
					<Switch
						id="allow-jellyfin"
						checked={manager.allowJellyfin}
						disabled={manager.status !== LoadStatus.READY}
						onCheckedChange={(value) => manager.setAllowJellyfin(value)}
					/>
				</div>
				{#if manager.signInError}
					<p class="text-xs text-destructive" role="alert">{manager.signInError}</p>
				{/if}
			</Card.Content>
		</Card.Root>
	{/if}
</div>
