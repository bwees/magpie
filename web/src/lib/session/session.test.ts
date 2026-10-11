import { describe, expect, it } from 'vitest';
import { AuthRoute, isAuthRoute, redirectFor, SessionStatus } from './session.svelte';

const HOME = '/';
const SETTINGS = '/settings/general';

describe('redirectFor', () => {
	it('sends everyone to the wizard until the admin exists', () => {
		expect(redirectFor(SessionStatus.SETUP_REQUIRED, HOME, false)).toBe(AuthRoute.SETUP);
		expect(redirectFor(SessionStatus.SETUP_REQUIRED, AuthRoute.SIGN_IN, false)).toBe(
			AuthRoute.SETUP
		);
		expect(redirectFor(SessionStatus.SETUP_REQUIRED, AuthRoute.SETUP, false)).toBeUndefined();
	});

	it('sends signed-out visitors to sign-in, including from the wizard', () => {
		expect(redirectFor(SessionStatus.SIGNED_OUT, SETTINGS, false)).toBe(AuthRoute.SIGN_IN);
		expect(redirectFor(SessionStatus.SIGNED_OUT, AuthRoute.SETUP, false)).toBe(AuthRoute.SIGN_IN);
		expect(redirectFor(SessionStatus.SIGNED_OUT, AuthRoute.SIGN_IN, false)).toBeUndefined();
	});

	it('keeps the wizard open only while it is running in this tab', () => {
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SETUP, true)).toBeUndefined();
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SETUP, false)).toBe(HOME);
	});

	it('moves signed-in users off the sign-in page and leaves app pages alone', () => {
		expect(redirectFor(SessionStatus.SIGNED_IN, AuthRoute.SIGN_IN, false)).toBe(HOME);
		expect(redirectFor(SessionStatus.SIGNED_IN, SETTINGS, false)).toBeUndefined();
	});

	it('waits while the session loads', () => {
		expect(redirectFor(SessionStatus.LOADING, SETTINGS, false)).toBeUndefined();
	});
});

describe('isAuthRoute', () => {
	it('matches only the pages without the shell', () => {
		expect(isAuthRoute(AuthRoute.SIGN_IN)).toBe(true);
		expect(isAuthRoute(AuthRoute.SETUP)).toBe(true);
		expect(isAuthRoute(SETTINGS)).toBe(false);
	});
});
