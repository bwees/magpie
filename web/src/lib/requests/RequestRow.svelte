<script lang="ts">
	import * as Avatar from '$lib/components/ui/avatar';
	import { Badge, type BadgeVariant } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { formatWhen } from '$lib/activity/format';
	import PosterImage from '$lib/catalog/PosterImage.svelte';
	import { titlePath } from '$lib/catalog/images';
	import type { RequestResponse } from '$lib/sdk';
	import { RequestStatus, initials, scopeLabel, statusLabel } from './requests';
	import type { RequestsPage } from './requests-page.svelte';

	let {
		request,
		page,
		profileName
	}: { request: RequestResponse; page: RequestsPage; profileName?: string } = $props();

	const STATUS_VARIANTS: Record<RequestStatus, BadgeVariant> = {
		[RequestStatus.PENDING]: 'outline',
		[RequestStatus.APPROVED]: 'secondary',
		[RequestStatus.PROCESSING]: 'secondary',
		[RequestStatus.PARTIALLY_AVAILABLE]: 'secondary',
		[RequestStatus.AVAILABLE]: 'default',
		[RequestStatus.DECLINED]: 'secondary',
		[RequestStatus.FAILED]: 'destructive'
	};

	const media = $derived(page.mediaOf(request));
	const name = $derived(page.titleOf(request));
	const status = $derived(request.status as RequestStatus);
	const label = $derived(statusLabel(request));
	const busy = $derived(page.busy.has(request.id));
	const scope = $derived(
		[scopeLabel(request.scope, request.seasons), profileName].filter(Boolean).join(' · ')
	);
	const note = $derived(request.declineReason || request.failureReason);
	const hasActions = $derived(
		page.canDecide(request) || page.canRetry(request) || page.canCancel(request)
	);
</script>

{#snippet requester()}
	<div class="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
		<Avatar.Root class="size-5">
			<Avatar.Fallback class="bg-muted text-[10px]">
				{initials(request.requester.displayName || '?')}
			</Avatar.Fallback>
		</Avatar.Root>
		<span class="truncate">{request.requester.displayName || 'Unknown user'}</span>
		<span class="shrink-0">· {formatWhen(request.createdAt)}</span>
	</div>
{/snippet}

<li class="flex flex-col gap-3 py-3 md:flex-row md:items-center md:gap-4">
	<div class="flex min-w-0 flex-1 items-start gap-3 md:items-center">
		<PosterImage src={media?.posterUrl} alt="" class="w-12 shrink-0" />
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2">
				{#if request.titleId}
					<a href={titlePath(request.titleId)} class="truncate font-medium hover:underline"
						>{name}</a
					>
				{:else}
					<span class="truncate font-medium">{name}</span>
				{/if}
				{#if media?.year}
					<span class="shrink-0 text-sm text-muted-foreground">{media.year}</span>
				{/if}
			</div>
			<p class="truncate text-sm text-muted-foreground">{scope}</p>
			{#if note}
				<p class="truncate text-xs text-muted-foreground" title={note}>{note}</p>
			{/if}
			<div class="mt-1 flex flex-col items-start gap-1.5 lg:hidden">
				<Badge variant={STATUS_VARIANTS[status]} class="md:hidden">{label}</Badge>
				{@render requester()}
			</div>
		</div>
	</div>

	<div class="hidden w-56 shrink-0 lg:block">{@render requester()}</div>
	<div class="hidden w-32 shrink-0 md:block">
		<Badge variant={STATUS_VARIANTS[status]}>{label}</Badge>
	</div>

	{#if hasActions}
		<div class="grid grid-cols-2 gap-2 md:flex md:w-48 md:shrink-0 md:justify-end">
			{#if page.canDecide(request)}
				<Button size="sm" disabled={busy} onclick={() => page.askToApprove(request)}>Approve</Button
				>
				<Button
					size="sm"
					variant="outline"
					disabled={busy}
					onclick={() => page.askToDecline(request)}
				>
					Decline
				</Button>
			{:else if page.canRetry(request)}
				<Button size="sm" class="col-span-2" disabled={busy} onclick={() => page.retry(request)}>
					Retry
				</Button>
			{:else if page.canCancel(request)}
				<Button
					size="sm"
					variant="outline"
					class="col-span-2"
					disabled={busy}
					onclick={() => page.cancel(request)}
				>
					Cancel request
				</Button>
			{/if}
		</div>
	{:else}
		<div class="hidden w-48 shrink-0 md:block"></div>
	{/if}
</li>
