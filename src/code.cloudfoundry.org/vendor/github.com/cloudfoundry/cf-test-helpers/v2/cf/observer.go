package cf

import (
	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
)

// Observer is exported here so downstream packages need not import .../internal.
type Observer = internal.CommandObserver

func RegisterObserver(observer Observer) {
	internal.RegisterObserver(observer)
}

func UnregisterObserver() {
	internal.UnregisterObserver()
}

func registeredObserver() Observer {
	return internal.RegisteredObserver()
}
