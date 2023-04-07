package cf

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Applications []Application
}

type Application struct {
	Buildpacks []string `yaml:",omitempty"`
	Command    string   `yaml:",omitempty"`
	Instances  int      `yaml:",omitempty"`
	Memory     string   `yaml:",omitempty"`
	Name       string
	Path       string              `yaml:",omitempty"`
	Routes     []map[string]string `yaml:",omitempty"`
}

var Push = func(appName string, args ...string) *gexec.Session {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		panic(err)
	}

	app := Application{
		Name: appName,
	}

	for i := 0; i < len(args); i += 2 {
		switch args[i] {
		case "-b":
			app.Buildpacks = append(app.Buildpacks, args[i+1])
		case "-c":
			app.Command = args[i+1]
		case "-d":
			app.Routes = append(app.Routes, map[string]string{"route": fmt.Sprintf("%s.%s", appName, args[i+1])})
		case "-i":
			instances, err := strconv.Atoi(args[i+1])
			if err != nil {
				panic(err)
			}
			app.Instances = instances
		case "-m":
			app.Memory = args[i+1]
		case "-p":
			path, err := filepath.Abs(args[i+1])
			if err != nil {
				panic(err)
			}
			app.Path = path
		}
	}

	manifest := Manifest{}

	manifest.Applications = append(manifest.Applications, app)

	manifestText, err := yaml.Marshal(manifest)
	if err != nil {
		panic(err)
	}

	manifestPath := filepath.Join(tmpDir, "manifest.yml")
	err = ioutil.WriteFile(manifestPath, manifestText, 0644)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"\n[%s]> Generated app manifest:\n%s\n",
		time.Now().UTC().Format("2006-01-02 15:04:05.00 (MST)"),
		string(manifestText),
	)

	return Cf("push",
		"-f", manifestPath,
	)
}
