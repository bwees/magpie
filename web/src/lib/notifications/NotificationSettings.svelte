<script lang="ts">
	import { onMount } from 'svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { LoadStatus } from '$lib/load-status';
	import { NotificationSettings } from './notification-settings.svelte';
	import NotificationUrlCard from './NotificationUrlCard.svelte';
	import {
		EXAMPLE_SCHEMES,
		MAX_NOTIFICATION_URLS,
		SDK_NOTIFICATIONS,
		SHOUTRRR_DOCS_URL
	} from './notification-source';

	let { admin }: { admin: boolean } = $props();

	// svelte-ignore state_referenced_locally
	const settings = new NotificationSettings(SDK_NOTIFICATIONS, admin);

	onMount(() => {
		settings.load();
	});
</script>

<div class="grid gap-4">
	<p class="text-sm text-muted-foreground">
		The bell always shows your notifications. Add a URL to also get them on Discord, Telegram, ntfy,
		Pushover, email and more, then tick the events each URL should get.
	</p>

	{#if settings.status === LoadStatus.ERROR}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Could not load notification settings</Alert.Title>
			<Alert.Description>
				<p>{settings.loadError}</p>
				<Button variant="outline" size="sm" class="mt-2" onclick={() => settings.load()}>
					Retry
				</Button>
			</Alert.Description>
		</Alert.Root>
	{:else if settings.status === LoadStatus.LOADING}
		<div class="grid gap-3" role="status" aria-label="Loading notification settings">
			<Skeleton class="h-40 w-full" />
			<Skeleton class="h-24 w-full" />
		</div>
	{:else}
		{#each settings.cards as card (card.url.id)}
			<NotificationUrlCard {card} {settings} />
		{/each}

		<Card.Root>
			<Card.Header>
				<Card.Title>Add a notification URL</Card.Title>
				<Card.Description>
					Paste a <a
						class="underline underline-offset-2 hover:text-foreground"
						href={SHOUTRRR_DOCS_URL}
						target="_blank"
						rel="noreferrer">shoutrrr</a
					>
					URL. Up to {MAX_NOTIFICATION_URLS} per account.
				</Card.Description>
			</Card.Header>
			<Card.Content>
				<form
					class="grid gap-3"
					onsubmit={(event) => {
						event.preventDefault();
						settings.add();
					}}
				>
					<div class="grid gap-3 sm:grid-cols-[minmax(0,12rem)_minmax(0,1fr)]">
						<div class="grid gap-1.5">
							<Label for="notification-new-label">Label</Label>
							<Input
								id="notification-new-label"
								bind:value={settings.newLabel}
								placeholder="Phone"
								disabled={settings.adding || settings.atLimit}
							/>
						</div>
						<div class="grid gap-1.5">
							<Label for="notification-new-url">URL</Label>
							<Input
								id="notification-new-url"
								bind:value={settings.newUrl}
								placeholder="discord://token@webhook-id"
								autocomplete="off"
								spellcheck={false}
								aria-invalid={settings.addError ? true : undefined}
								aria-describedby="notification-new-url-help"
								disabled={settings.adding || settings.atLimit}
							/>
						</div>
					</div>
					<div id="notification-new-url-help" class="flex flex-wrap items-center gap-1.5">
						<span class="text-xs text-muted-foreground">For example</span>
						{#each EXAMPLE_SCHEMES as scheme (scheme)}
							<button
								type="button"
								class="rounded-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
								onclick={() => settings.useScheme(scheme)}
							>
								<Badge variant="secondary" class="font-mono">{scheme}</Badge>
							</button>
						{/each}
					</div>
					{#if settings.addError}
						<p class="text-sm break-words text-destructive" role="alert">{settings.addError}</p>
					{/if}
					{#if settings.atLimit}
						<p class="text-sm text-muted-foreground">
							You have {MAX_NOTIFICATION_URLS} URLs. Delete one to add another.
						</p>
					{/if}
					<div class="flex justify-end">
						<Button
							type="submit"
							disabled={settings.adding || settings.atLimit || settings.newUrl.trim() === ''}
						>
							{settings.adding ? 'Adding…' : 'Add URL'}
						</Button>
					</div>
				</form>
			</Card.Content>
		</Card.Root>
	{/if}
</div>
