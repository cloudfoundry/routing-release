package workflowhelpers

import (
	"time"
)

type userContext interface {
	SetCfHomeDir() (string, string)
	UnsetCfHomeDir(string, string)
	Login()
	Logout()
	TargetSpace()
}

func AsUser(uc userContext, timeout time.Duration, actions func()) {
	originalCfHomeDir, currentCfHomeDir := uc.SetCfHomeDir()
	uc.Login()
	defer func() {
		uc.Logout()
		uc.UnsetCfHomeDir(originalCfHomeDir, currentCfHomeDir)
	}()

	uc.TargetSpace()
	actions()
}
