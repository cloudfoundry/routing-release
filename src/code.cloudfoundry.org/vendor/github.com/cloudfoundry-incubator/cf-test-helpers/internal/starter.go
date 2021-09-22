package internal

import "github.com/onsi/gomega/gexec"

type Starter interface {
	Start(Reporter, string, ...string) (*gexec.Session, error)
}
