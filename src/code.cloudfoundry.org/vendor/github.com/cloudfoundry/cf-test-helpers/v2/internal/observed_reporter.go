package internal

// redactorProvider lets a secret-bearing base reporter (e.g. RedactingReporter)
// hand its redactor to the seam; without it the observer would see the secret
// in the clear.
type redactorProvider interface {
	Redactor() Redactor
}

// observedReporter returns base unchanged when no observer is registered, so
// default behavior is byte-for-byte preserved.
func observedReporter(base Reporter) Reporter {
	observer := RegisteredObserver()
	if observer == nil {
		return base
	}
	return NewTeeReporter(base, NewObservingReporter(observer, deriveRedactor(base)))
}

func ObservedReporter(base Reporter) Reporter {
	return observedReporter(base)
}

func deriveRedactor(base Reporter) Redactor {
	if p, ok := base.(redactorProvider); ok {
		return p.Redactor()
	}
	return NewRedactor()
}
