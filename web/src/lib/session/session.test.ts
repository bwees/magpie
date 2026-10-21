import { describe, expect, it } from 'vitest';
import { NavRole } from '$lib/navigation/nav';
import { AuthRoute, isAuthRoute, redirectFor, SessionStatus } from './session.svelte';

const HOME = '/';
const SETTINGS = '/settings/general';
const NOTIFICATIONS = '/settings/notifications';
const ADMIN = NavRole.ADMIN;
const REQUESTER = NavRole.REQUESTER;

describe('redirectFor', () => {
	it('sends everyone to the wizard until the admin exists', () => {
		expect(redirectFor(SessionStatus.SETUP_REQUIRED, HOME, false, ADMIN)).toBe(AuthRoute.SETUP);
		expect(redirectFor(SessionStatus.SETUP_REQUIRED, AuthRoute.SIGN_IN, false, ADMIN)).toBe(
			AuthRoute.SETUP
		);
		expect(
			redirectFor(SessionStatus.SETUP_REQUIRED, AuthRoute.SETUP, false, ADMIN)
		).toBeUndefined();
	});

	it('sends signed-out visitors to sign-in, including from the wizard', () => {
		expect(redirectFor(SessionStatus.SIGNED_OUT, SETTINGS, false, ADMIN)).toBe(AuthRoute.SIGN_IN);
		expect(redirectFor(SessionStatus.SIGNED_OUT, AuthRoute.SETUP, false, ADMIN)).toBe(
			AuthRoute.SIGN_IN
		);
		expect(redirectFor(SessionStatus.SIGNED_OUT, AuthRoute.SIGN_IN, false, ADMIN)).toBeUndefined();
	});

	it('keeps the wizard open only while it is running in this tab', () => {
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SETUP, true, ADMIN)).toBeUndefined();
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SETUP, false, ADMIN)).toBe(HOME);
	});

	it('moves signed-in users off the sign-in page and leaves app pages alone', () => {
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SIGN_IN, false, ADMIN)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, SETTINGS, false, ADMIN)).toBeUndefined();
	});

	it('sends requesters home from admin-only pages but keeps notification settings', () => {
		expect(redirectFor(SessionStatus.SIGNED_IN, SETTINGS, false, REQUESTER)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, '/movies', false, REQUESTER)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, NOTIFICATIONS, false, REQUESTER)).toBeUndefined();
		expect(redirectFor(SessionStatus.SIGNED_IN, '/requests', false, REQUESTER)).toBeUndefined();
	});

	it('lets file managers open Activity but not its admin sub-pages', () => {
		const manager = NavRole.FILE_MANAGER;
		expect(redirectFor(SessionStatus.SIGNED_IN, '/activity', false, manager)).toBeUndefined();
		expect(redirectFor(SessionStatus.SIGNED_IN, '/activity/agents', false, manager)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, SETTINGS, false, manager)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, '/activity', false, REQUESTER)).toBe(HOME);
	});

	it('waits while the session loads', () => {
		expect(redirectFor(SessionStatus.LOADING, SETTINGS, false, REQUESTER)).toBeUndefined();
	});
});

describe('isAuthRoute', () => {
	it('matches only the pages without the shell', () => {
		expect(isAuthRoute(AuthRoute.SIGN_IN)).toBe(true);
		expect(isAuthRoute(AuthRoute.SETUP)).toBe(true);
		expect(isAuthRoute(SETTINGS)).toBe(false);
	});
});
