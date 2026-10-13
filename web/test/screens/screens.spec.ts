import { expect, test, type Locator, type Page, type TestInfo } from '@playwright/test';
import { FakeApi } from './fake-api';
import { MOVIE_ID, NOW, SERIES_ID } from './fake-data';

const PHONE_PROJECT = 'phone';
const NARROWEST_PHONE_PX = 360;
const MIN_TOUCH_TARGET_PX = 40;

interface Screen {
	name: string;
	path: string;
	/** Steps after the page loads, such as opening a tab or a dialog. */
	open?: (page: Page, phone: boolean) => Promise<void>;
	/** Dialogs cover the viewport, so they are captured without scrolling the page. */
	viewportOnly?: boolean;
}

/** Waits for an opening dialog's zoom, so sizes are measured at full scale. */
async function settleAnimations(target: Locator): Promise<void> {
	await target.evaluate((element) =>
		Promise.all(
			element
				.getAnimations({ subtree: true })
				.filter((animation) => animation.effect?.getTiming().iterations !== Infinity)
				.map((animation) => animation.finished)
		)
	);
}

async function openInteractiveSearch(page: Page): Promise<void> {
	await page.getByRole('button', { name: 'Interactive search' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toBeVisible();
	await settleAnimations(dialog);
}

async function openTitleSearch(page: Page, phone: boolean): Promise<void> {
	if (phone) {
		await page.getByRole('button', { name: 'Search', exact: true }).click();
		await settleAnimations(page.getByRole('dialog'));
	} else {
		await page.getByRole('searchbox', { name: 'Search titles' }).click();
	}
	await expect(page.getByText('Trending this week')).toBeVisible();
}

const SCREENS: Screen[] = [
	{ name: 'home', path: '/' },
	{ name: 'movie-title', path: `/titles/${MOVIE_ID}` },
	{ name: 'series-title', path: `/titles/${SERIES_ID}` },
	{ name: 'movies', path: '/movies' },
	{ name: 'tv', path: '/tv' },
	{ name: 'requests', path: '/requests' },
	{ name: 'requests-issues', path: '/requests?tab=issues' },
	{ name: 'issue-thread', path: '/requests?issue=I1', viewportOnly: true },
	{ name: 'activity-downloads', path: '/activity' },
	{
		name: 'activity-agents',
		path: '/activity',
		open: (page) => page.getByRole('tab', { name: 'Agents' }).click()
	},
	{
		name: 'activity-gpu-jobs',
		path: '/activity',
		open: (page) => page.getByRole('tab', { name: 'GPU jobs' }).click()
	},
	{ name: 'settings', path: '/settings/indexers' },
	{ name: 'settings-transcode', path: '/settings/transcode' },
	{
		name: 'interactive-search',
		path: `/titles/${MOVIE_ID}`,
		open: openInteractiveSearch,
		viewportOnly: true
	},
	{ name: 'title-search', path: '/', open: openTitleSearch, viewportOnly: true }
];

function isPhone(info: TestInfo): boolean {
	return info.project.name === PHONE_PROJECT;
}

async function visit(page: Page, path: string): Promise<void> {
	await page.goto(path);
	await page.waitForLoadState('networkidle');
}

async function expectTouchSized(target: Locator): Promise<void> {
	const box = await target.boundingBox();
	expect(box?.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
}

let api: FakeApi;

test.beforeEach(async ({ page }) => {
	api = new FakeApi();
	await page.clock.setFixedTime(new Date(NOW));
	await api.install(page);
});

test.afterEach(() => {
	expect(api.unhandled, 'requests the fake API has no answer for').toEqual([]);
});

for (const screen of SCREENS) {
	test(`${screen.name} matches its screenshot`, async ({ page }, info) => {
		await visit(page, screen.path);
		await screen.open?.(page, isPhone(info));
		await page.waitForLoadState('networkidle');
		await expect(page).toHaveScreenshot(`${screen.name}.png`, {
			fullPage: !screen.viewportOnly
		});
	});
}

test('leaving a title page by a link loads the next page', async ({ page }) => {
	const errors: Error[] = [];
	page.on('pageerror', (error) => errors.push(error));
	await visit(page, `/titles/${MOVIE_ID}`);

	await page.locator('a[href="/tv"]:visible').first().click();
	await expect(page).toHaveURL('/tv');
	await expect(page.getByText('Severance').first()).toBeVisible();

	await page
		.getByRole('link', { name: /Severance/ })
		.first()
		.click();
	await expect(page).toHaveURL(`/titles/${SERIES_ID}`);
	await expect(page.getByRole('heading', { name: 'Severance' }).first()).toBeVisible();

	await page.locator('a[href="/"]:visible').first().click();
	await expect(page).toHaveURL('/');
	await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
	expect(errors).toEqual([]);
});

test('the tracker pill opens a list of trackers on tap', async ({ page }, info) => {
	test.skip(!isPhone(info), 'phones have no hover');
	await visit(page, `/titles/${MOVIE_ID}`);
	await openInteractiveSearch(page);
	await page.getByRole('button', { name: /Tracker status/ }).tap();
	await expect(page.getByText('PassThePopcorn')).toBeVisible();
	await expect(page).toHaveScreenshot('interactive-search-trackers.png');
});

test('no screen scrolls sideways at 360 px', async ({ page }, info) => {
	test.skip(!isPhone(info), 'phone layout only');
	await page.setViewportSize({
		width: NARROWEST_PHONE_PX,
		height: page.viewportSize()?.height ?? 0
	});
	for (const screen of SCREENS) {
		await visit(page, screen.path);
		await screen.open?.(page, true);
		const overflow = await page.evaluate(
			() => document.documentElement.scrollWidth - document.documentElement.clientWidth
		);
		expect(overflow, screen.name).toBe(0);
	}
});

test('nav tabs, primary actions and grab buttons are touch sized', async ({ page }, info) => {
	test.skip(!isPhone(info), 'phone layout only');
	await visit(page, '/requests');
	for (const tab of await page.getByRole('navigation', { name: 'Main' }).getByRole('link').all()) {
		await expectTouchSized(tab);
	}
	await expectTouchSized(page.getByRole('button', { name: 'Approve' }).first());
	await expectTouchSized(page.getByRole('button', { name: 'Decline' }).first());

	await visit(page, `/titles/${MOVIE_ID}`);
	await expectTouchSized(page.getByRole('button', { name: 'Search', exact: true }).last());
	await openInteractiveSearch(page);
	await expectTouchSized(page.getByRole('button', { name: 'Grab' }).first());
});

test('the phone search opens full screen with the input focused', async ({ page }, info) => {
	test.skip(!isPhone(info), 'phone layout only');
	await visit(page, '/');
	await openTitleSearch(page, true);
	await expect(page.getByRole('dialog').getByRole('searchbox')).toBeFocused();
	await expect(page.getByRole('button', { name: 'Close search' })).toBeVisible();
});

test('the phone avatar menu has Settings and Sign out', async ({ page }, info) => {
	test.skip(!isPhone(info), 'phone layout only');
	await visit(page, '/');
	await page.getByRole('button', { name: 'Account menu' }).click();
	await expect(page.getByRole('menuitem', { name: 'Settings' })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: 'Sign out' })).toBeVisible();
});
