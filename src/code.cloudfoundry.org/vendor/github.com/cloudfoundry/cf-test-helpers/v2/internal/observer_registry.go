package internal

import "sync"

var (
	observerMu      sync.RWMutex
	currentObserver CommandObserver
)

func RegisterObserver(observer CommandObserver) {
	observerMu.Lock()
	defer observerMu.Unlock()
	currentObserver = observer
}

func UnregisterObserver() {
	observerMu.Lock()
	defer observerMu.Unlock()
	currentObserver = nil
}

func RegisteredObserver() CommandObserver {
	observerMu.RLock()
	defer observerMu.RUnlock()
	return currentObserver
}
