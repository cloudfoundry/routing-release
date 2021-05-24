package locket

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/consuladapter"
	"github.com/hashicorp/consul/api"
)

type LostLockError string

func (e LostLockError) Error() string {
	return fmt.Sprintf("Lost lock '%s'", string(e))
}

var ErrInvalidSession = errors.New("invalid session")
var ErrDestroyed = errors.New("already destroyed")
var ErrCancelled = errors.New("cancelled")

type Session struct {
	client consuladapter.Client

	name     string
	ttl      time.Duration
	noChecks bool

	errCh chan error

	lock      sync.Mutex
	id        string
	destroyed bool
	doneCh    chan struct{}
	lostLock  string
}

func NewSession(sessionName string, ttl time.Duration, client consuladapter.Client) (*Session, error) {
	return newSession(sessionName, ttl, false, client)
}

func NewSessionNoChecks(sessionName string, ttl time.Duration, client consuladapter.Client) (*Session, error) {
	return newSession(sessionName, ttl, true, client)
}

func newSession(sessionName string, ttl time.Duration, noChecks bool, client consuladapter.Client) (*Session, error) {
	doneCh := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	s := &Session{
		client:   client,
		name:     sessionName,
		ttl:      ttl,
		noChecks: noChecks,
		doneCh:   doneCh,
		errCh:    errCh,
	}

	return s, nil
}

func (s *Session) ID() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.id
}

func (s *Session) Err() chan error {
	return s.errCh
}

func (s *Session) Destroy() {
	s.lock.Lock()
	s.destroy()
	s.lock.Unlock()
}

// Lock must be held
func (s *Session) destroy() {
	if s.destroyed == false {
		close(s.doneCh)

		if s.id != "" {
			s.client.Session().Destroy(s.id, nil)
		}

		s.destroyed = true
	}
}

// Lock must be held
func (s *Session) createSession() error {
	if s.destroyed {
		return ErrDestroyed
	}

	if s.id != "" {
		return nil
	}

	se := &api.SessionEntry{
		Name:      s.name,
		Behavior:  api.SessionBehaviorDelete,
		TTL:       s.ttl.String(),
		LockDelay: 1 * time.Nanosecond,
	}

	id, renewTTL, err := create(se, s.noChecks, s.client)
	if err != nil {
		return err
	}

	s.id = id

	go func() {
		err := s.client.Session().RenewPeriodic(renewTTL, id, nil, s.doneCh)
		s.lock.Lock()
		lostLock := s.lostLock
		s.destroy()
		s.lock.Unlock()

		if lostLock != "" {
			err = LostLockError(lostLock)
		} else {
			err = convertError(err)
		}
		s.errCh <- err
	}()

	return err
}

func (s *Session) Recreate() (*Session, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	session, err := newSession(s.name, s.ttl, s.noChecks, s.client)
	if err != nil {
		return nil, err
	}

	err = session.createSession()
	if err != nil {
		return nil, err
	}

	return session, err
}

func (s *Session) AcquireLock(key string, value []byte) error {
	s.lock.Lock()
	err := s.createSession()
	s.lock.Unlock()
	if err != nil {
		return err
	}

	lockOptions := api.LockOptions{
		Key:              key,
		Value:            value,
		Session:          s.id,
		MonitorRetries:   int(s.ttl / MonitorRetryTime),
		MonitorRetryTime: MonitorRetryTime,
	}

	lock, err := s.client.LockOpts(&lockOptions)
	if err != nil {
		return convertError(err)
	}

	lostCh, err := lock.Lock(s.doneCh)
	if err != nil {
		return convertError(err)
	}
	if lostCh == nil {
		return ErrCancelled
	}

	go func() {
		select {
		case <-lostCh:
			s.lock.Lock()
			defer s.lock.Unlock()

			if s.destroyed {
				s.errCh <- LostLockError(key)
				return
			}

			s.lostLock = key
			s.destroy()
		case <-s.doneCh:
		}
	}()

	return nil
}

func (s *Session) SetPresence(key string, value []byte) (<-chan string, error) {
	s.lock.Lock()
	err := s.createSession()
	s.lock.Unlock()
	if err != nil {
		return nil, err
	}

	lockOptions := api.LockOptions{
		Key:              key,
		Value:            value,
		Session:          s.id,
		MonitorRetries:   int(s.ttl / MonitorRetryTime),
		MonitorRetryTime: MonitorRetryTime,
	}

	lock, err := s.client.LockOpts(&lockOptions)
	if err != nil {
		return nil, convertError(err)
	}

	lostCh, err := lock.Lock(s.doneCh)
	if err != nil {
		return nil, convertError(err)
	}
	if lostCh == nil {
		return nil, ErrCancelled
	}

	presenceLost := make(chan string, 1)
	go func() {
		select {
		case <-lostCh:
			presenceLost <- key
		case <-s.doneCh:
		}
	}()

	return presenceLost, nil
}

func create(se *api.SessionEntry, noChecks bool, client consuladapter.Client) (string, string, error) {
	session := client.Session()
	agent := client.Agent()

	nodeName, err := agent.NodeName()
	if err != nil {
		return "", "", err
	}

	nodeSessions, _, err := session.Node(nodeName, nil)
	if err != nil {
		return "", "", err
	}

	sessions := findSessions(se.Name, nodeSessions)
	if sessions != nil {
		for _, s := range sessions {
			_, err = session.Destroy(s.ID, nil)
			if err != nil {
				return "", "", err
			}
		}
	}

	var f func(*api.SessionEntry, *api.WriteOptions) (string, *api.WriteMeta, error)
	if noChecks {
		f = session.CreateNoChecks
	} else {
		f = session.Create
	}

	id, _, err := f(se, nil)
	if err != nil {
		return "", "", err
	}

	return id, se.TTL, nil
}

func findSessions(name string, sessions []*api.SessionEntry) []*api.SessionEntry {
	var matches []*api.SessionEntry
	for _, session := range sessions {
		if session.Name == name {
			matches = append(matches, session)
		}
	}

	return matches
}

func convertError(err error) error {
	if err == nil {
		return err
	}

	if strings.Contains(err.Error(), "500 (Invalid session)") {
		return ErrInvalidSession
	}

	return err
}
