<script lang="ts">
	import User from '@lucide/svelte/icons/user';
	import LogOut from '@lucide/svelte/icons/log-out';
	import * as Avatar from '$lib/components/ui/avatar';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { NAV_ITEMS } from '$lib/navigation/nav';
	import { getSession } from '$lib/session/session.svelte';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
	const session = getSession();
	const settings = NAV_ITEMS.SETTINGS;
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger class="rounded-full" aria-label="Account menu">
		<Avatar.Root class="size-8">
			<Avatar.Fallback class="bg-primary text-primary-foreground">
				<User class="size-4" />
			</Avatar.Fallback>
		</Avatar.Root>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="w-44">
		{#if shell.isMobile}
			<DropdownMenu.Item>
				{#snippet child({ props })}
					<a href={settings.route} {...props}>
						<settings.icon />
						{settings.label}
					</a>
				{/snippet}
			</DropdownMenu.Item>
			<DropdownMenu.Separator />
		{/if}
		{#if session.user}
			<DropdownMenu.Label class="truncate">{session.user.displayName}</DropdownMenu.Label>
			<DropdownMenu.Separator />
		{/if}
		<DropdownMenu.Item onclick={() => session.signOut()}>
			<LogOut />
			Sign out
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
