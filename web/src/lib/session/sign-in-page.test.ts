import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import type { MeResponse } from '$lib/sdk';
import {
	QUICK_CONNECT_POLL_MS,
	QuickConnectStatus,
	QuickConnectStep,
	SignInErrorKind,
	SignInMode,
	SignInPage
} from './sign-in-page.svelte';

const sdk = vi.hoisted(() => ({
	getSignInMethods: vi.fn(),
	signInJellyfin: vi.fn(),
	signInLocal: vi.fn(),
	startQuickConnect: vi.fn(),
	pollQuickConnect: vi.fn()
}));

vi.mock('$lib/sdk', async () => {
	const { ProblemType } = await vi.importActual<typeof import('$lib/sdk')>('$lib/sdk');
	return {
		...sdk,
		ProblemType,
		apiProblemType: (error: { data?: { type?: string } }) => error?.data?.type,
		apiErrorMessage: (error: { data?: { detail?: string } }) => error?.data?.detail ?? 'failed'
	};
});

const USER: MeResponse = {
	id: 'u1',
	username: 'alice',
	displayName: 'Alice',
	role: 'REQUESTER',
	localAdmin: false,
	permissions: {
		autoApproveMovies: false,
		autoApproveTv: false,
		viewOthersRequests: false,
		reportIssues: true,
		manageFiles: false
	}
};
const ALL_METHODS = { jellyfin: true, quickConnect: true, jellyfinReachable: true };

function problem(type: string, detail = '') {
	return { data: { type: `urn:magpie:problem:${type}`, detail } };
}

describe('SignInPage', () => {
	let signedIn: Mock<(user: MeResponse) => void>;
	let page: SignInPage;

	beforeEach(() => {
		vi.resetAllMocks();
		vi.useFakeTimers();
		signedIn = vi.fn<(user: MeResponse) => void>();
		page = new SignInPage(signedIn);
		sdk.getSignInMethods.mockResolvedValue(ALL_METHODS);
	});

	afterEach(() => {
		page.stop();
		vi.useRealTimers();
	});

	it('starts on the Jellyfin form when Jellyfin sign-in is on', async () => {
		await page.load();
		expect(page.mode).toBe(SignInMode.JELLYFIN);
	});

	it('falls back to the local admin when Jellyfin sign-in is off or methods fail', async () => {
		sdk.getSignInMethods.mockResolvedValue({ ...ALL_METHODS, jellyfin: false });
		await page.load();
		expect(page.mode).toBe(SignInMode.LOCAL);

		sdk.getSignInMethods.mockRejectedValue(new Error('down'));
		await page.load();
		expect(page.mode).toBe(SignInMode.LOCAL);
	});

	it('signs in through Jellyfin or the local admin depending on the mode', async () => {
		await page.load();
		sdk.signInJellyfin.mockResolvedValue(USER);
		page.username = ' alice ';
		page.password = 'pw';
		await page.submit();
		expect(sdk.signInJellyfin).toHaveBeenCalledWith({ username: 'alice', password: 'pw' });
		expect(signedIn).toHaveBeenCalledWith(USER);

		page.useLocalAdmin();
		sdk.signInLocal.mockResolvedValue(USER);
		page.username = 'admin';
		page.password = 'pw';
		await page.submit();
		expect(sdk.signInLocal).toHaveBeenCalledWith({ username: 'admin', password: 'pw' });
	});

	it.each([
		['invalid-credentials', SignInErrorKind.INVALID_CREDENTIALS],
		['jellyfin-unavailable', SignInErrorKind.JELLYFIN_UNAVAILABLE],
		['user-disabled', SignInErrorKind.DISABLED],
		['jellyfin-user-disabled', SignInErrorKind.DISABLED],
		['too-many-sign-ins', SignInErrorKind.OTHER]
	])('shows %s as %s and clears the password', async (type, kind) => {
		await page.load();
		sdk.signInJellyfin.mockRejectedValue(problem(type));
		page.username = 'alice';
		page.password = 'pw';
		await page.submit();
		expect(page.error?.kind).toBe(kind);
		expect(page.password).toBe('');
		expect(signedIn).not.toHaveBeenCalled();
	});

	it('shows the Quick Connect code and polls until approved', async () => {
		await page.load();
		sdk.startQuickConnect.mockResolvedValue({ code: '123456', handle: 'h1', expiresAt: '' });
		sdk.pollQuickConnect
			.mockResolvedValueOnce({ status: QuickConnectStatus.PENDING })
			.mockResolvedValueOnce({ status: QuickConnectStatus.APPROVED, user: USER });

		await page.startQuickConnect();
		expect(page.mode).toBe(SignInMode.QUICK_CONNECT);
		expect(page.quickConnectCode).toBe('123456');
		expect(page.quickConnectStep).toBe(QuickConnectStep.WAITING);

		await vi.advanceTimersByTimeAsync(QUICK_CONNECT_POLL_MS);
		expect(signedIn).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(QUICK_CONNECT_POLL_MS);
		expect(sdk.pollQuickConnect).toHaveBeenCalledWith('h1');
		expect(signedIn).toHaveBeenCalledWith(USER);
	});

	it('marks an expired code and stops polling', async () => {
		await page.load();
		sdk.startQuickConnect.mockResolvedValue({ code: '123456', handle: 'h1', expiresAt: '' });
		sdk.pollQuickConnect.mockRejectedValue(problem('quick-connect-expired'));

		await page.startQuickConnect();
		await vi.advanceTimersByTimeAsync(QUICK_CONNECT_POLL_MS * 3);
		expect(page.quickConnectStep).toBe(QuickConnectStep.EXPIRED);
		expect(sdk.pollQuickConnect).toHaveBeenCalledTimes(1);
	});

	it('stops polling when the user goes back to the password form', async () => {
		await page.load();
		sdk.startQuickConnect.mockResolvedValue({ code: '123456', handle: 'h1', expiresAt: '' });
		await page.startQuickConnect();

		page.useJellyfin();
		await vi.advanceTimersByTimeAsync(QUICK_CONNECT_POLL_MS * 2);
		expect(sdk.pollQuickConnect).not.toHaveBeenCalled();
	});
});
