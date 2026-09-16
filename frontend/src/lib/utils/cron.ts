export type CronFeedback = {
	description: string;
	error: string;
};

const weekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const weekdayNames: Record<string, number> = {
	SUN: 0,
	MON: 1,
	TUE: 2,
	WED: 3,
	THU: 4,
	FRI: 5,
	SAT: 6
};
const monthNames: Record<string, number> = {
	JAN: 1,
	FEB: 2,
	MAR: 3,
	APR: 4,
	MAY: 5,
	JUN: 6,
	JUL: 7,
	AUG: 8,
	SEP: 9,
	OCT: 10,
	NOV: 11,
	DEC: 12
};

function fieldValue(value: string, names?: Record<string, number>): number | null {
	if (/^\d+$/.test(value)) return Number(value);
	return names?.[value.toUpperCase()] ?? null;
}

function validateField(
	expression: string,
	min: number,
	max: number,
	label: string,
	names?: Record<string, number>
): string {
	if (!expression) return `${label} is required.`;

	for (const item of expression.split(',')) {
		const stepParts = item.split('/');
		if (stepParts.length > 2) return `${label} has an invalid step.`;
		if (stepParts.length === 2) {
			const step = Number(stepParts[1]);
			if (!Number.isInteger(step) || step < 1) return `${label} step must be a positive number.`;
		}

		const range = stepParts[0];
		if (range === '*') continue;
		const bounds = range.split('-');
		if (bounds.length > 2) return `${label} has an invalid range.`;
		const values = bounds.map((value) => fieldValue(value, names));
		if (values.some((value) => value === null)) return `${label} contains an invalid value.`;
		if (values.some((value) => value! < min || value! > max)) {
			return `${label} must be between ${min} and ${max}.`;
		}
		if (values.length === 2 && values[0]! > values[1]!) return `${label} range must run from low to high.`;
	}

	return '';
}

function formatTime(hour: number, minute: number): string {
	const period = hour < 12 ? 'AM' : 'PM';
	const displayHour = hour % 12 || 12;
	return `${displayHour}:${minute.toString().padStart(2, '0')} ${period}`;
}

function numericField(value: string, names?: Record<string, number>): number | null {
	if (value.includes(',') || value.includes('-') || value.includes('/') || value === '*') return null;
	return fieldValue(value, names);
}

export function describeCron(value: string): CronFeedback {
	const expression = value.trim();
	if (!expression) return { description: '', error: 'Enter a five-field cron schedule.' };

	const fields = expression.split(/\s+/);
	if (fields.length !== 5) {
		return { description: '', error: 'Use five fields: minute hour day month weekday.' };
	}

	const checks = [
		validateField(fields[0], 0, 59, 'Minute'),
		validateField(fields[1], 0, 23, 'Hour'),
		validateField(fields[2], 1, 31, 'Day of month'),
		validateField(fields[3], 1, 12, 'Month', monthNames),
		validateField(fields[4], 0, 6, 'Weekday', weekdayNames)
	];
	const error = checks.find(Boolean) ?? '';
	if (error) return { description: '', error };

	const [minuteField, hourField, dayField, monthField, weekdayField] = fields;
	if (fields.every((field) => field === '*')) return { description: 'Every minute', error: '' };

	const minute = numericField(minuteField);
	const hour = numericField(hourField);
	if (minute !== null && hourField === '*' && dayField === '*' && monthField === '*' && weekdayField === '*') {
		return { description: minute === 0 ? 'Every hour' : `Every hour at :${minute.toString().padStart(2, '0')}`, error: '' };
	}
	if (minute !== null && hour !== null && dayField === '*' && monthField === '*') {
		const time = formatTime(hour, minute);
		if (weekdayField === '*') return { description: `Every day at ${time}`, error: '' };
		const weekday = numericField(weekdayField, weekdayNames);
		if (weekday !== null) return { description: `Every ${weekdays[weekday]} at ${time}`, error: '' };
		const weekdayList = weekdayField.split(',').map((item) => numericField(item, weekdayNames));
		if (weekdayList.every((item) => item !== null)) {
			const names = weekdayList.map((item) => weekdays[item!]);
			const joined = names.length === 2 ? names.join(' and ') : `${names.slice(0, -1).join(', ')}, and ${names.at(-1)}`;
			return { description: `Every ${joined} at ${time}`, error: '' };
		}
	}
	if (minute !== null && hour !== null && monthField === '*' && weekdayField === '*') {
		const day = numericField(dayField);
		if (day !== null) return { description: `On day ${day} of every month at ${formatTime(hour, minute)}`, error: '' };
	}

	return { description: 'Custom schedule', error: '' };
}
