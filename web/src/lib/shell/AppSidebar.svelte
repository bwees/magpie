<script lang="ts">
	import { page } from '$app/state';
	import Bird from '@lucide/svelte/icons/bird';
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { ownsPath, Route, SIDEBAR_GROUPS } from '$lib/navigation/nav';
	import { getShell } from './shell-state.svelte';

	const shell = getShell();
</script>

<Sidebar.Root collapsible="icon">
	<Sidebar.Header>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton size="lg">
					{#snippet child({ props })}
						<a href={Route.HOME} {...props}>
							<span
								class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground"
							>
								<Bird class="size-4" />
							</span>
							<span class="font-semibold">Magpie</span>
						</a>
					{/snippet}
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Header>

	<Sidebar.Content>
		{#each SIDEBAR_GROUPS as group (group.label)}
			<Sidebar.Group>
				<Sidebar.GroupLabel>{group.label}</Sidebar.GroupLabel>
				<Sidebar.Menu>
					{#each group.items as item (item.route)}
						{@const badge = shell.badgeFor(item)}
						<Sidebar.MenuItem>
							<Sidebar.MenuButton
								class="data-active:text-sidebar-primary"
								tooltipContent={item.label}
								isActive={ownsPath(item, page.url.pathname)}
							>
								{#snippet child({ props })}
									<a href={item.route} {...props}>
										<item.icon />
										<span>{item.label}</span>
									</a>
								{/snippet}
							</Sidebar.MenuButton>
							{#if badge}
								<Sidebar.MenuBadge class="bg-primary text-primary-foreground"
									>{badge}</Sidebar.MenuBadge
								>
							{/if}
						</Sidebar.MenuItem>
					{/each}
				</Sidebar.Menu>
			</Sidebar.Group>
		{/each}
	</Sidebar.Content>

	<Sidebar.Footer>
		<Sidebar.Trigger />
	</Sidebar.Footer>
	<Sidebar.Rail />
</Sidebar.Root>
