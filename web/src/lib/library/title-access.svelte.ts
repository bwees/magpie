import { getTitleAccess, type TitleAccessResponse } from '$lib/sdk';

export type TitleAccessApi = (title: string) => Promise<TitleAccessResponse>;

/** Whether the viewer may manage a title's files. The server decides, since it knows who requested the title. */
export class TitleAccess {
	manageFiles = $state(false);

	readonly #title: string;
	readonly #api: TitleAccessApi;

	/** Admins start allowed so their controls don't flash in. */
	constructor(title: string, admin: boolean, api: TitleAccessApi = getTitleAccess) {
		this.#title = title;
		this.#api = api;
		this.manageFiles = admin;
	}

	/** A failed check keeps the role's default; the server still refuses what the viewer can't do. */
	async load(): Promise<void> {
		try {
			this.manageFiles = (await this.#api(this.#title)).manageFiles;
		} catch {
			return;
		}
	}
}
