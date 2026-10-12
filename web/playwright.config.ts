import { defineConfig } from '@playwright/test';

const PORT = 4317;
const BASE_URL = `http://127.0.0.1:${PORT}`;
const PHONE = { width: 390, height: 844 };
const TABLET = { width: 820, height: 1180 };

export default defineConfig({
	testDir: 'test/screens',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	reporter: 'list',
	snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{arg}{ext}',
	expect: {
		toHaveScreenshot: { animations: 'disabled', caret: 'hide', maxDiffPixelRatio: 0.01 }
	},
	use: {
		baseURL: BASE_URL,
		colorScheme: 'dark',
		locale: 'en-US',
		timezoneId: 'UTC'
	},
	projects: [
		{ name: 'phone', use: { viewport: PHONE, hasTouch: true, isMobile: true } },
		{ name: 'tablet', use: { viewport: TABLET, hasTouch: true } }
	],
	webServer: {
		// exec hands the shell's pid to vite, so Playwright's stop signal reaches the server.
		command: `vite build && exec vite preview --host 127.0.0.1 --port ${PORT} --strictPort`,
		url: BASE_URL,
		reuseExistingServer: false,
		timeout: 180_000
	}
});
