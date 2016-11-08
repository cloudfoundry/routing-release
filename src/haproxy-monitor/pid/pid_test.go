package pid_test

import (
	"errors"
	"haproxy-monitor/pid"
	"haproxy-monitor/pid/fakes"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pid", func() {
	var (
		fileLock *fakes.FakeFileLock
	)

	BeforeEach(func() {
		fileLock = &fakes.FakeFileLock{}
	})

	Context("when it can not acquire the lock", func() {
		BeforeEach(func() {
			fileLock.LockReturns(errors.New("lock error"))
		})

		It("returns an error and -1", func() {
			p, err := pid.GetPid(fileLock)
			Expect(err).To(HaveOccurred())
			Expect(p).To(Equal(-1))
		})

		It("retries the lock on error", func() {
			_, err := pid.GetPid(fileLock)
			Expect(err).To(HaveOccurred())

			Expect(fileLock.LockCallCount()).To(Equal(3))
		})
	})

	Context("when it acquire the lock", func() {
		var (
			testFile *os.File
		)

		BeforeEach(func() {
			var err error
			testFile, err = ioutil.TempFile(os.TempDir(), "pid")
			Expect(err).NotTo(HaveOccurred())

			Expect(ioutil.WriteFile(testFile.Name(), []byte("9"), 0600)).To(Succeed())
			fileLock.NameReturns(testFile.Name())
		})

		AfterEach(func() {
			Expect(os.Remove(testFile.Name())).To(Succeed())
		})

		It("calls unlock once successfully locked", func() {
			p, err := pid.GetPid(fileLock)
			Expect(err).NotTo(HaveOccurred())
			Expect(p).To(Equal(9))

			Expect(fileLock.UnlockCallCount()).To(Equal(1))
		})

		Context("when it fails to read the file", func() {
			BeforeEach(func() {
				fileLock.NameReturns("non-existent")
			})

			It("returns an error and -1", func() {
				p, err := pid.GetPid(fileLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Cannot read file"))
				Expect(p).To(Equal(-1))
			})
		})

		Context("when it cannot convert contents to an integer", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(testFile.Name(), []byte("words"), 0600)).To(Succeed())
			})

			It("returns an error and -1", func() {
				p, err := pid.GetPid(fileLock)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Cannot convert file to integer"))
				Expect(p).To(Equal(-1))
			})
		})
	})
})
