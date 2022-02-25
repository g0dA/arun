package command

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/docker/docker/pkg/stringid"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/specconv"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

//InitSandboxConfig init config
func InitSandboxConfig(id string, options execOptions) (*specs.Spec, *configs.Config, error) {

	u, err := user.Lookup(options.user)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	specMount := []specs.Mount{
		{
			Destination: "/",
			Type:        "rbind",
			Source:      "/",
			Options:     []string{"rbind", "rw", "private"},
		},
		{
			Destination: "/dev",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
		},
		{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"},
		},
		{
			Destination: "/dev/shm",
			Type:        "tmpfs",
			Source:      "shm",
			Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
		},
		{
			Destination: "/dev/mqueue",
			Type:        "mqueue",
			Source:      "mqueue",
			Options:     []string{"nosuid", "noexec", "nodev"},
		},
	}

	spec := &specs.Spec{
		Version: specs.Version,
		Root: &specs.Root{
			Path:     "/tmp",
			Readonly: false,
		},
		Process: &specs.Process{
			Terminal: true,
			User: specs.User{
				UID: uint32(uid),
				GID: uint32(gid),
			},
			Args:            options.command,
			Env:             []string{"PATH=/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin", "TERM=xterm"},
			Cwd:             "/tmp",
			NoNewPrivileges: false,
			Capabilities:    &options.specConfig.Capabilities,
			Rlimits: []specs.POSIXRlimit{
				{
					Type: "RLIMIT_NOFILE",
					Hard: uint64(1024),
					Soft: uint64(1024),
				},
			},
		},
		Hostname: "",
		Mounts:   specMount,
		Linux: &specs.Linux{
			MaskedPaths: []string{
				"/proc/acpi",
				"/proc/asound",
				"/proc/kcore",
				"/proc/keys",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_Sandbox",
				"/sys/firmware",
				"/proc/scsi",
			},
			ReadonlyPaths: append([]string{
				"/proc/bus",
				"/proc/fs",
				"/proc/irq",
				"/proc/sys",
				"/proc/sysrq-trigger",
			}, options.specConfig.Ropath...),
			Namespaces: []specs.LinuxNamespace{
				{
					Type: specs.MountNamespace,
				},
			},
		},
	}

	config, err := specconv.CreateLibcontainerConfig(&specconv.CreateOpts{
		CgroupName:       id,
		UseSystemdCgroup: false,
		NoPivotRoot:      false,
		NoNewKeyring:     false,
		Spec:             spec,
		RootlessEUID:     os.Geteuid() != 0,
		RootlessCgroups:  false,
	})
	if err != nil {
		return spec, config, errors.WithStack(err)
	}

	return spec, config, nil
}

// loadFactory returns the configured factory instance for execing containers.
func (cli *SandboxCli) loadFactory() (libcontainer.Factory, error) {

	return libcontainer.New("/var/lib/sandbox/containerd", libcontainer.Cgroupfs, libcontainer.InitArgs(os.Args[0], "init"))
}

//CreateSandboxContainer instabce of create Sandbox container
func (cli *SandboxCli) CreateSandboxContainer(options execOptions) (*specs.Spec, libcontainer.Container, error) {

	containerID := stringid.GenerateRandomID()
	spec, config, err := InitSandboxConfig(containerID, options)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	factory, err := cli.loadFactory()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	//return container status stopped
	container, err := factory.Create(containerID, config)
	if err != nil {
		container.Destroy()
		return nil, nil, errors.WithStack(err)
	}

	// write config path to containerRoot
	containerRoot, err := securejoin.SecureJoin("/var/lib/sandbox/containerd", containerID)
	if err != nil {
		return nil, nil, err
	}
	absoluteConfigPath, err := filepath.Abs(options.config)
	if err != nil {
		return nil, nil, err
	}
	err = ioutil.WriteFile(containerRoot+"/config", []byte(absoluteConfigPath), 0644)
	if err != nil {
		return nil, nil, err
	}
	return spec, container, nil
}

//CleanSandboxContainer clean all Sandbox container
func (cli *SandboxCli) CleanSandboxContainer(c libcontainer.Container) error {
	err := c.Destroy()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
