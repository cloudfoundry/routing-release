package pid

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:generate counterfeiter -o fakes/fake_file_lock.go . FileLock
type FileLock interface {
	Name() string
	Lock() error
	Unlock() error
}

type fileLock struct {
	*os.File
}

func NewFileLock(file *os.File) FileLock {
	return &fileLock{file}
}

func (l *fileLock) Lock() error {
	return syscall.Flock(int(l.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func (l *fileLock) Unlock() error {
	return syscall.Flock(int(l.Fd()), syscall.LOCK_UN)
}

func GetPid(file FileLock) (int, error) {
	var err error

	retries := 3
	for i := 0; i < retries; i++ {
		err = file.Lock()
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return -1, fmt.Errorf("Cannot acquire lock: %s", err.Error())
	}
	defer file.Unlock()

	fileBytes, err := ioutil.ReadFile(file.Name())
	if err != nil {
		return -1, fmt.Errorf("Cannot read file: %s", err.Error())
	}

	data := strings.TrimSpace(string(fileBytes))
	pid, err := strconv.Atoi(data)
	if err != nil {
		return -1, fmt.Errorf("Cannot convert file to integer: Contents: %s", string(fileBytes))
	}

	return pid, nil
}
