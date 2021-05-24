package utils_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/testutil"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fileutils", func() {
	Describe("WriteToFile", func() {
		Context("when valid path is passed", func() {
			var (
				fileName    string
				fileContent string
			)

			BeforeEach(func() {
				fileName = testutil.RandomFileName("fixtures/file_", "")
				fileContent = "some content"
			})

			AfterEach(func() {
				err := os.Remove(fileName)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when valid content is passed", func() {
				It("writes to destination file", func() {
					err := utils.WriteToFile([]byte(fileContent), fileName)
					Expect(err).ShouldNot(HaveOccurred())
					actualContent, err := ioutil.ReadFile(fileName)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(string(actualContent)).To(Equal(fileContent))
				})
			})

			Context("when empty content is passed", func() {
				It("create an empty file", func() {
					err := utils.WriteToFile(nil, fileName)
					Expect(err).ShouldNot(HaveOccurred())
					actualContent, err := ioutil.ReadFile(fileName)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(actualContent).Should(HaveLen(0))
				})
			})
		})

		Context("when invalid file path", func() {
			It("returns error", func() {
				invalidFileName := testutil.RandomFileName("fixtures-invalid-path/file_", "")
				err := utils.WriteToFile([]byte("some content"), invalidFileName)
				Expect(err).Should(HaveOccurred())
			})
		})
	})

	Describe("CopyFile", func() {
		Context("when source file exist ", func() {
			Context("when destinaiton file is valid", func() {

				var (
					fileName string
				)

				BeforeEach(func() {
					fileName = testutil.RandomFileName("fixtures/file_", "")
				})

				AfterEach(func() {
					err := os.Remove(fileName)
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("copies to destination file ", func() {
					srcFileName := "fixtures/test_file"
					err := utils.CopyFile(srcFileName, fileName)
					Expect(err).ShouldNot(HaveOccurred())

					expectedContent, err := ioutil.ReadFile(srcFileName)
					Expect(err).ShouldNot(HaveOccurred())

					actualContent, err := ioutil.ReadFile(fileName)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(actualContent).To(Equal(expectedContent))
				})
			})
			Context("when destinaiton file is invalid", func() {
				It("returns error", func() {
					err := utils.CopyFile("fixtures/test_file", "fixtures-does-not-exist/file_does_not_exist")
					Expect(err).Should(HaveOccurred())
				})
			})
		})

		Context("when source file does not exist ", func() {

			It("returns error", func() {
				err := utils.CopyFile("fixtures-does-not-exist/file_does_not_exist", "fixtures/destination-file")
				Expect(err).Should(HaveOccurred())
			})
		})

	})

	Describe("FileExists", func() {
		Context("when file exists", func() {
			It("it returns true", func() {
				Expect(utils.FileExists("fixtures/test_file")).To(Equal(true))
			})
		})
		Context("when file does not exists", func() {
			It("it returns false", func() {
				Expect(utils.FileExists("fixtures/non_existing_test_file")).To(Equal(false))
			})
		})

	})
})
