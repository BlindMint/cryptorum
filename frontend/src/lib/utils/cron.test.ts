import { describe, expect, it } from 'vitest';
import { describeCron } from './cron';

describe('describeCron', () => {
	it('describes a weekly backup schedule', () => {
		expect(describeCron('0 4 * * 1')).toEqual({
			description: 'Every Monday at 4:00 AM',
			error: ''
		});
	});

	it('describes daily and hourly schedules', () => {
		expect(describeCron('30 21 * * *').description).toBe('Every day at 9:30 PM');
		expect(describeCron('15 * * * *').description).toBe('Every hour at :15');
	});

	it('identifies an out-of-range weekday', () => {
		expect(describeCron('* * * * 9')).toEqual({
			description: '',
			error: 'Weekday must be between 0 and 6.'
		});
	});

	it('explains the required field count', () => {
		expect(describeCron('0 4 * *').error).toBe('Use five fields: minute hour day month weekday.');
	});

	it('accepts named weekdays', () => {
		expect(describeCron('0 4 * * MON').description).toBe('Every Monday at 4:00 AM');
	});
});
