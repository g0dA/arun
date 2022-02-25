package command

import (
	"encoding/json"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"io/ioutil"
	"os"
	"runtime"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()
		unmountPaths()
		factory, _ := libcontainer.New("")
		if err := factory.StartInitialization(); err != nil {
			logrus.Fatal(err)
		}
		panic("--this line should have never been executed, congratulations--")
	}
}

func unmountPaths() {
	//make mount point "/" private
	flag := unix.MS_PRIVATE
	_ = unix.Mount("", "/", "", uintptr(flag), "")
	//get the config path
	containerRoot := os.Getenv("_LIBCONTAINER_STATEDIR")
	if containerRoot == "" {
		return
	}
	configPath := containerRoot + "/config"
	file, err := os.Open(configPath)
	if err != nil {
		panic(err)
	}

	defer file.Close()
	cp, err := ioutil.ReadAll(file)
	var options execOptions
	cf, err := os.Open(string(cp))
	if err != nil {
		panic(err)
	}
	defer cf.Close()
	err = json.NewDecoder(cf).Decode(&options.specConfig)
	if err != nil {
		panic(err)
	}
	// execute unmount
	for _, unmountPath := range options.specConfig.UnmountPaths {
		_ = unix.Unmount(unmountPath, 0)
	}

}
