import { ALERT_SEVERITIES } from './pipeline';

const WHEN = new Intl.DateTimeFormat(undefined, {
	month: 'short',
	day: 'numeric',
	hour: 'numeric',
	minute: '2-digit'
});

export function formatWhen(at: string): string {
	return WHEN.format(new Date(at));
}

/** Rejections, failures and nothing-accepted searches get the failure styling. */
export function isAlert(severity: string): boolean {
	return ALERT_SEVERITIES.includes(severity);
}
