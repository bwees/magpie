<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import Bell from '@lucide/svelte/icons/bell';
	import BellOff from '@lucide/svelte/icons/bell-off';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import { toast } from 'svelte-sonner';
	import { formatWhen } from '$lib/activity/format';
	import { Button } from '$lib/components/ui/button';
	import * as Popover from '$lib/components/ui/popover';
	import { Separator } from '$lib/components/ui/separator';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { getLive } from '$lib/live/live-client.svelte';
	import { LoadStatus } from '$lib/load-status';
	import { NOTIFICATION_SETTINGS_PATH } from '$lib/navigation/nav';
	import type { NotificationResponse } from '$lib/sdk';
	import { cn } from '$lib/utils';
	import { NotificationInbox } from './notification-inbox.svelte';
	import { SDK_NOTIFICATIONS } from './notification-source';

	const SKELETON_ROWS = ['first', 'second', 'third'];

	const inbox = new NotificationInbox(SDK_NOTIFICATIONS);
	const live = getLive();
	let open = $state(false);

	onMount(() => {
		void inbox.load();
		return inbox.connect(live, (created) => toast(created.title));
	});

	function follow(item: NotificationResponse): void {
		open = false;
		void inbox.markRead(item);
		if (item.link) void goto(item.link);
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button
				{...props}
				variant="ghost"
				size="icon"
				class="relative rounded-full"
				aria-label={inbox.hasUnread
					? `Notifications, ${inbox.unreadCount} unread`
					: 'Notifications'}
			>
				<Bell class="size-4" />
				{#if inbox.hasUnread}
					<span class="absolute top-2 right-2 size-2 rounded-full bg-primary" aria-hidden="true"
					></span>
				{/if}
			</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content align="end" class="w-[calc(100vw-2rem)] max-w-sm gap-0 p-0">
		<div class="flex items-center justify-between gap-2 px-4 py-3">
			<h2 class="text-sm font-medium">Notifications</h2>
			<Button
				variant="ghost"
				size="sm"
				class="h-7 text-xs"
				disabled={!inbox.hasUnread}
				onclick={() => inbox.markAllRead()}
			>
				Mark all read
			</Button>
		</div>
		<Separator />
		<div class="max-h-[min(28rem,60vh)] overflow-y-auto">
			{#if inbox.status === LoadStatus.LOADING}
				<div class="grid gap-3 p-4" role="status" aria-label="Loading notifications">
					{#each SKELETON_ROWS as key (key)}
						<Skeleton class="h-10 w-full" />
					{/each}
				</div>
			{:else if inbox.status === LoadStatus.ERROR}
				<div class="flex flex-col items-center gap-2 p-6 text-center">
					<CircleAlert class="size-5 text-destructive" />
					<p class="text-sm text-muted-foreground">{inbox.error}</p>
					<Button variant="outline" size="sm" onclick={() => inbox.load()}>Retry</Button>
				</div>
			{:else if inbox.empty}
				<div class="flex flex-col items-center gap-2 p-6 text-center">
					<BellOff class="size-5 text-muted-foreground" />
					<p class="text-sm text-muted-foreground">No notifications</p>
				</div>
			{:else}
				<ul>
					{#each inbox.items as item (item.id)}
						<li>
							<button
								type="button"
								class="flex w-full gap-3 px-4 py-3 text-left hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
								onclick={() => follow(item)}
							>
								<span
									class={cn(
										'mt-1.5 size-2 shrink-0 rounded-full',
										item.read ? 'bg-transparent' : 'bg-primary'
									)}
									aria-hidden="true"
								></span>
								<span class="grid min-w-0 gap-0.5">
									<span class={cn('text-sm break-words', !item.read && 'font-medium')}>
										{item.title}
										{#if !item.read}<span class="sr-only">(unread)</span>{/if}
									</span>
									{#if item.body}
										<span class="line-clamp-2 text-xs break-words text-muted-foreground">
											{item.body}
										</span>
									{/if}
									<time class="text-xs text-muted-foreground" datetime={item.createdAt}>
										{formatWhen(item.createdAt)}
									</time>
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
		<Separator />
		<a
			href={NOTIFICATION_SETTINGS_PATH}
			class="block px-4 py-2.5 text-center text-xs text-muted-foreground hover:text-foreground"
			onclick={() => (open = false)}
		>
			Notification settings
		</a>
	</Popover.Content>
</Popover.Root>
