import {
	apiErrorMessage,
	type NotificationKindResponse,
	type NotificationUrlChangeRequest,
	type NotificationUrlResponse
} from '$lib/sdk';
import { LoadStatus } from '$lib/load-status';
import { KindScope, MAX_NOTIFICATION_URLS, type NotificationSource } from './notification-source';

export enum TestOutcome {
	SENT = 'SENT',
	FAILED = 'FAILED'
}

export interface KindGroup {
	label: string;
	kinds: NotificationKindResponse[];
}

/** Splits the kinds a role may hold into the groups the checkboxes are shown in, dropping empty ones. */
export function groupKinds(kinds: NotificationKindResponse[], admin: boolean): KindGroup[] {
	const groups: KindGroup[] = [
		{
			label: admin ? 'Requests and issues (any user)' : 'Your requests and issues',
			kinds: kinds.filter((kind) => kind.scope === KindScope.REQUESTER)
		},
		{
			label: 'Needs attention',
			kinds: kinds.filter((kind) => kind.scope === KindScope.ADMIN && !kind.optIn)
		},
		{ label: 'Pipeline successes', kinds: kinds.filter((kind) => kind.optIn) }
	];
	return groups.filter((group) => group.kinds.length > 0);
}

/** One URL card: the stored URL, its label draft, and the state of its last change and test. */
export class NotificationUrlCard {
	url = $state() as NotificationUrlResponse;
	label = $state('');
	saving = $state(false);
	testing = $state(false);
	error = $state<string>();
	testOutcome = $state<TestOutcome>();
	testMessage = $state('');

	constructor(url: NotificationUrlResponse) {
		this.url = url;
		this.label = url.label;
	}

	get busy(): boolean {
		return this.saving || this.testing;
	}

	get labelChanged(): boolean {
		return this.label.trim() !== this.url.label;
	}

	subscribes(kind: string): boolean {
		return this.url.kinds.includes(kind);
	}
}

/** State for Settings → Notifications: the user's URLs, the kinds they may pick, and the add form. */
export class NotificationSettings {
	status = $state(LoadStatus.LOADING);
	loadError = $state('');
	kinds = $state<NotificationKindResponse[]>([]);
	cards = $state<NotificationUrlCard[]>([]);
	newLabel = $state('');
	newUrl = $state('');
	adding = $state(false);
	addError = $state<string>();

	readonly #source: NotificationSource;
	readonly #admin: boolean;

	constructor(source: NotificationSource, admin: boolean) {
		this.#source = source;
		this.#admin = admin;
	}

	get groups(): KindGroup[] {
		return groupKinds(this.kinds, this.#admin);
	}

	get atLimit(): boolean {
		return this.cards.length >= MAX_NOTIFICATION_URLS;
	}

	async load(): Promise<void> {
		this.status = LoadStatus.LOADING;
		try {
			const [kinds, urls] = await Promise.all([this.#source.kinds(), this.#source.urls()]);
			this.kinds = kinds;
			this.cards = urls.map((url) => new NotificationUrlCard(url));
			this.status = LoadStatus.READY;
		} catch (error) {
			this.loadError = apiErrorMessage(error, 'Could not load your notification settings.');
			this.status = LoadStatus.ERROR;
		}
	}

	/** Prefills the add field with a scheme when it is empty. */
	useScheme(scheme: string): void {
		if (this.newUrl.trim() === '') this.newUrl = scheme;
	}

	async add(): Promise<void> {
		const url = this.newUrl.trim();
		if (url === '' || this.atLimit) return;
		this.adding = true;
		this.addError = undefined;
		try {
			const added = await this.#source.addUrl({ label: this.newLabel.trim(), url, kinds: [] });
			this.cards.push(new NotificationUrlCard(added));
			this.newLabel = '';
			this.newUrl = '';
		} catch (error) {
			this.addError = apiErrorMessage(error, 'Could not add that URL.');
		} finally {
			this.adding = false;
		}
	}

	setKind(card: NotificationUrlCard, kind: string, on: boolean): Promise<void> {
		const kinds = on
			? [...card.url.kinds, kind]
			: card.url.kinds.filter((existing) => existing !== kind);
		return this.#update(card, { kinds });
	}

	saveLabel(card: NotificationUrlCard): Promise<void> {
		if (!card.labelChanged) return Promise.resolve();
		return this.#update(card, { label: card.label.trim(), kinds: null });
	}

	async remove(card: NotificationUrlCard): Promise<void> {
		card.saving = true;
		card.error = undefined;
		try {
			await this.#source.deleteUrl(card.url.id);
			this.cards = this.cards.filter((existing) => existing !== card);
		} catch (error) {
			card.error = apiErrorMessage(error, 'Could not delete this URL.');
		} finally {
			card.saving = false;
		}
	}

	async test(card: NotificationUrlCard): Promise<void> {
		card.testing = true;
		card.testOutcome = undefined;
		try {
			await this.#source.testUrl(card.url.id);
			card.testOutcome = TestOutcome.SENT;
			card.testMessage = 'Test sent. Check the service.';
		} catch (error) {
			card.testOutcome = TestOutcome.FAILED;
			card.testMessage = apiErrorMessage(error, 'The test message failed.');
		} finally {
			card.testing = false;
		}
	}

	async #update(card: NotificationUrlCard, change: NotificationUrlChangeRequest): Promise<void> {
		const previous = card.url;
		card.url = {
			...previous,
			kinds: change.kinds ?? previous.kinds,
			label: change.label ?? previous.label
		};
		card.saving = true;
		card.error = undefined;
		try {
			card.url = await this.#source.updateUrl(previous.id, change);
			card.label = card.url.label;
		} catch (error) {
			card.url = previous;
			card.label = previous.label;
			card.error = apiErrorMessage(error, 'Could not save this change.');
		} finally {
			card.saving = false;
		}
	}
}
