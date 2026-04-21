type FullscreenOptionsWithNavigation = FullscreenOptions & {
	navigationUI?: 'auto' | 'hide' | 'show';
};

const TRUE_FULLSCREEN_CLASS = 'true-fullscreen-reader';
let fullscreenChangeListenerBound = false;

function setTrueFullscreenClass(enabled: boolean) {
	document.documentElement.classList.toggle(TRUE_FULLSCREEN_CLASS, enabled);
	document.body.classList.toggle(TRUE_FULLSCREEN_CLASS, enabled);
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

export async function toggleReaderFullscreen(useStandardFullscreen = false) {
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
