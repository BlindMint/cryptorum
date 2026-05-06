type FullscreenOptionsWithNavigation = FullscreenOptions & {
	navigationUI?: 'auto' | 'hide' | 'show';
};

const TRUE_FULLSCREEN_CLASS = 'true-fullscreen';
let fullscreenChangeListenerBound = false;
let fullscreenStateListenerBound = false;
const fullscreenStateSubscribers = new Set<(enabled: boolean) => void>();

function setTrueFullscreenClass(enabled: boolean) {
	document.documentElement.classList.toggle(TRUE_FULLSCREEN_CLASS, enabled);
	document.body.classList.toggle(TRUE_FULLSCREEN_CLASS, enabled);
	fullscreenStateSubscribers.forEach((subscriber) => subscriber(enabled));
}

function bindFullscreenChangeListener() {
	if (fullscreenChangeListenerBound) return;
	fullscreenChangeListenerBound = true;
	document.addEventListener('fullscreenchange', () => {
		if (!document.fullscreenElement) {
			setTrueFullscreenClass(false);
		}
	});
}

function bindFullscreenStateListener() {
	if (fullscreenStateListenerBound || typeof document === 'undefined') return;
	fullscreenStateListenerBound = true;
	document.addEventListener('fullscreenchange', () => {
		fullscreenStateSubscribers.forEach((subscriber) => subscriber(Boolean(document.fullscreenElement)));
	});
}

export function subscribeFullscreenState(callback: (enabled: boolean) => void) {
	if (typeof document === 'undefined') {
		callback(false);
		return () => {};
	}
	bindFullscreenStateListener();
	fullscreenStateSubscribers.add(callback);
	callback(Boolean(document.fullscreenElement));
	return () => fullscreenStateSubscribers.delete(callback);
}

export async function toggleAppFullscreen(useStandardFullscreen = false) {
	if (typeof document === 'undefined') return;
	bindFullscreenChangeListener();

	if (document.fullscreenElement) {
		setTrueFullscreenClass(false);
		await document.exitFullscreen();
		return;
	}

	setTrueFullscreenClass(!useStandardFullscreen);

	try {
		const options: FullscreenOptionsWithNavigation | undefined = useStandardFullscreen
			? undefined
			: { navigationUI: 'hide' };
		await document.documentElement.requestFullscreen(options);
	} catch (error) {
		setTrueFullscreenClass(false);
		throw error;
	}
}

export async function enterAppFullscreen(useStandardFullscreen = false) {
	if (typeof document === 'undefined' || document.fullscreenElement) return;
	bindFullscreenChangeListener();

	setTrueFullscreenClass(!useStandardFullscreen);

	try {
		const options: FullscreenOptionsWithNavigation | undefined = useStandardFullscreen
			? undefined
			: { navigationUI: 'hide' };
		await document.documentElement.requestFullscreen(options);
	} catch (error) {
		setTrueFullscreenClass(false);
		throw error;
	}
}

export async function toggleReaderFullscreen(useStandardFullscreen = false) {
	return toggleAppFullscreen(useStandardFullscreen);
}

export async function enterReaderFullscreen(useStandardFullscreen = false) {
	return enterAppFullscreen(useStandardFullscreen);
}
