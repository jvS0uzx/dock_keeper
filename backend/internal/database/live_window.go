package database

import "time"

const MinLiveWindow = 30 * time.Second

func LiveWindowFor(intervalSec int) time.Duration {
	if intervalSec <= 0 {
		return MinLiveWindow
	}
	if w := time.Duration(intervalSec) * 3 * time.Second; w > MinLiveWindow {
		return w
	}
	return MinLiveWindow
}
