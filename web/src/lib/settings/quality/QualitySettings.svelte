<script lang="ts">
	import { onMount } from 'svelte';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import X from '@lucide/svelte/icons/x';
	import { toast } from 'svelte-sonner';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Select from '$lib/components/ui/select';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { Switch } from '$lib/components/ui/switch';
	import { LoadStatus } from '$lib/load-status';
	import PresetCards from './PresetCards.svelte';
	import ProfileMatrix from './ProfileMatrix.svelte';
	import { BLOCK_SCORE, formatSyncedAt, LIMITS } from './quality';
	import { QualitySettingsManager } from './quality-manager.svelte';

	const SKELETON_CARDS = ['720', '1080', '2160'];

	const manager = new QualitySettingsManager((message) => toast.success(message));
	const now = new Date();

	const profileName = (id: string | undefined) =>
		manager.profiles.find((profile) => profile.id === id)?.name ?? 'Pick a profile';

	onMount(() => {
		manager.load();
	});
</script>

<div class="grid gap-8">
	<p class="text-sm text-muted-foreground">
		Which releases Magpie grabs, and when a file is good enough. Profiles are shared by movies and
		TV.
	</p>

	{#if manager.status === LoadStatus.LOADING}
		<div class="grid gap-3 sm:grid-cols-3">
			{#each SKELETON_CARDS as key (key)}
				<Skeleton class="h-44 w-full" />
			{/each}
		</div>
	{:else if manager.status === LoadStatus.ERROR}
		<Alert.Root variant="destructive">
			<CircleAlert />
			<Alert.Title>Couldn't load quality settings</Alert.Title>
			<Alert.Description class="flex flex-wrap items-center gap-3">
				{manager.error}
				<Button size="sm" variant="outline" onclick={() => manager.load()}>Retry</Button>
			</Alert.Description>
		</Alert.Root>
	{:else if manager.settings}
		{@const settings = manager.settings}
		<section class="grid gap-3">
			<h3 class="text-sm font-semibold">Presets</h3>
			<PresetCards {manager} />
		</section>

		<form
			class="grid gap-4"
			onsubmit={(event) => {
				event.preventDefault();
				manager.saveSettings().then((saved) => saved && toast.success('Quality settings saved'));
			}}
		>
			<h3 class="text-sm font-semibold">Defaults</h3>
			<div class="grid gap-4 sm:grid-cols-3">
				<div class="grid gap-1.5">
					<Label for="default-profile">Default profile for new requests</Label>
					<Select.Root
						type="single"
						value={settings.defaultProfileId}
						onValueChange={(value) => (settings.defaultProfileId = value)}
					>
						<Select.Trigger id="default-profile" class="w-full">
							{profileName(settings.defaultProfileId)}
						</Select.Trigger>
						<Select.Content>
							{#each manager.profiles as profile (profile.id)}
								<Select.Item value={profile.id} label={profile.name} />
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="grid gap-1.5">
					<Label for="delay">Delay before grabbing</Label>
					<div class="flex items-center gap-2">
						<Input
							id="delay"
							type="number"
							class="w-24"
							min={LIMITS.DELAY_MINUTES.MIN}
							max={LIMITS.DELAY_MINUTES.MAX}
							bind:value={settings.defaultDelayMinutes}
						/>
						<span class="text-sm text-muted-foreground">minutes</span>
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="settle">Search settle window</Label>
					<div class="flex items-center gap-2">
						<Input
							id="settle"
							type="number"
							class="w-24"
							min={LIMITS.SETTLE_SECONDS.MIN}
							max={LIMITS.SETTLE_SECONDS.MAX}
							bind:value={settings.settleWindowSeconds}
						/>
						<span class="text-sm text-muted-foreground">seconds</span>
					</div>
				</div>
			</div>
			<p class="text-xs text-muted-foreground">
				The delay waits for a better release unless one meets the cutoff or reaches the profile's
				top score. Automatic grabs wait the settle window for slower indexers.
			</p>
			<div class="flex items-center gap-3">
				<Button type="submit" size="sm" disabled={manager.savingSettings}>Save</Button>
				{#if manager.settingsError}
					<span class="text-sm text-destructive" role="alert">{manager.settingsError}</span>
				{/if}
			</div>
		</form>

		<Card.Root>
			<Card.Content class="grid gap-3">
				<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<div>
						<div class="text-sm font-medium">TRaSH Guides sync</div>
						<div class="text-xs text-muted-foreground">
							Daily · {formatSyncedAt(manager.trash?.lastSyncedAt, now)} · your score changes are kept
						</div>
					</div>
					<div class="flex items-center gap-3">
						<Button
							size="sm"
							variant="outline"
							disabled={manager.syncing}
							onclick={() => manager.syncNow()}
						>
							<RefreshCw class={manager.syncing ? 'animate-spin' : ''} />
							{manager.syncing ? 'Syncing…' : 'Sync now'}
						</Button>
						<Switch
							checked={settings.trashSyncEnabled}
							onCheckedChange={(checked) => manager.setTrashSync(checked)}
							aria-label="Sync TRaSH Guides daily"
						/>
					</div>
				</div>
				{#if manager.trashError}
					<p class="text-sm text-destructive" role="alert">{manager.trashError}</p>
				{/if}
			</Card.Content>
		</Card.Root>

		<form
			class="grid gap-2"
			onsubmit={(event) => {
				event.preventDefault();
				manager.addBlockTerm();
			}}
		>
			<Label for="block-term">Block term</Label>
			<p class="text-xs text-muted-foreground">
				Never grab releases whose name contains this text. It becomes a custom format scored {BLOCK_SCORE}
				in every profile.
			</p>
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					id="block-term"
					class="sm:max-w-xs"
					placeholder="e.g. HC.Subs"
					bind:value={manager.term}
				/>
				<Button type="submit" size="sm" variant="outline" disabled={manager.blocking}>Block</Button>
			</div>
			{#if manager.blockError}
				<p class="text-sm text-destructive" role="alert">{manager.blockError}</p>
			{/if}
		</form>

		<Collapsible.Root bind:open={manager.advancedOpen}>
			<Collapsible.Trigger>
				{#snippet child({ props })}
					<Button {...props} variant="ghost" size="sm" class="-ml-2">
						Advanced: profiles and custom formats
						<ChevronDown class={manager.advancedOpen ? 'rotate-180' : ''} />
					</Button>
				{/snippet}
			</Collapsible.Trigger>
			<Collapsible.Content class="mt-4 grid gap-8">
				<section class="grid gap-3">
					<h3 class="text-sm font-semibold">Profiles</h3>
					<div class="flex flex-wrap gap-2">
						{#each manager.profiles as profile (profile.id)}
							<Button
								size="sm"
								variant={manager.editor?.profile.id === profile.id ? 'secondary' : 'outline'}
								onclick={() => manager.openProfile(profile.id)}
							>
								{profile.name}
								{#if profile.trashProfileId}<Badge variant="outline">TRaSH</Badge>{/if}
							</Button>
						{/each}
					</div>
					{#if manager.editorError}
						<p class="text-sm text-destructive" role="alert">{manager.editorError}</p>
					{/if}
					{#if manager.editorLoading}
						<Skeleton class="h-64 w-full" />
					{:else if manager.editor}
						{@const editor = manager.editor}
						<Card.Root class="min-w-0">
							<Card.Content class="min-w-0">
								{#key editor}
									<ProfileMatrix
										{editor}
										isDefault={editor.profile.id === manager.defaultProfileId}
										onDelete={() => manager.removeProfile(editor.profile.id)}
									/>
								{/key}
							</Card.Content>
						</Card.Root>
					{:else}
						<p class="text-sm text-muted-foreground">
							Pick a profile to edit its qualities and scores.
						</p>
					{/if}
				</section>

				<section class="grid gap-3">
					<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
						<h3 class="text-sm font-semibold">
							Custom formats
							<span class="font-normal text-muted-foreground">· {manager.formats.length}</span>
						</h3>
						<Input
							class="h-8 sm:max-w-56"
							placeholder="Filter formats…"
							aria-label="Filter custom formats"
							bind:value={manager.formatQuery}
						/>
					</div>
					{#if manager.formats.length === 0}
						<p class="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
							No custom formats yet. Sync TRaSH Guides to get its formats, or block a term above.
						</p>
					{:else}
						<div class="max-h-96 divide-y overflow-y-auto rounded-lg border">
							{#each manager.filteredFormats as format (format.id)}
								<div class="flex items-center gap-2 px-3 py-2 text-sm">
									<span class="min-w-0 flex-1 truncate">{format.name}</span>
									{#if format.trashId}
										<Badge variant="outline">TRaSH</Badge>
									{/if}
									<span class="hidden text-xs text-muted-foreground sm:inline">
										{format.specs.length}
										{format.specs.length === 1 ? 'condition' : 'conditions'}
									</span>
									<Button
										size="icon"
										variant="ghost"
										class="size-7"
										aria-label="Delete {format.name}"
										onclick={() => manager.removeFormat(format.id)}
									>
										<X />
									</Button>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			</Collapsible.Content>
		</Collapsible.Root>
	{/if}
</div>
