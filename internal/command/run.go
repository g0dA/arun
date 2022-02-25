package command

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/opencontainers/runc/libcontainer/utils"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const signalBufferSize = 2048

type signalHandler struct {
	signals      chan os.Signal
	notifySocket *notifySocket
}

type notifySocket struct {
	socket     *net.UnixConn
	host       string
	socketPath string
}

type exit struct {
	pid    int
	status int
}

// newSignalHandler returns a signal handler for processing SIGCHLD and SIGWINCH signals
// while still forwarding all other signals to the process.
// If notifySocket is present, use it to read systemd notifications from the container and
// forward them to notifySocketHost.
func newSignalHandler(enableSubreaper bool) *signalHandler {
	if enableSubreaper {
		// set us as the subreaper before registering the signal handler for the container
		if err := system.SetSubreaper(1); err != nil {
			logrus.Warn(err)
		}
	}
	// ensure that we have a large buffer size so that we do not miss any signals
	// in case we are not processing them fast enough.
	s := make(chan os.Signal, signalBufferSize)
	// handle all signals for the process.
	signal.Notify(s)
	return &signalHandler{
		signals:      s,
		notifySocket: nil,
	}
}

// forward handles the main signal event loop forwarding, resizing, or reaping depending
// on the signal received.
func (h *signalHandler) forward(process *libcontainer.Process, tty *tty, detach bool) error {
	// make sure we know the pid of our main process so that we can return
	// after it dies.
	if detach && h.notifySocket == nil {
		return nil
	}

	pid1, err := process.Pid()
	if err != nil {
		return err
	}
	// Perform the initial tty resize. Always ignore errors resizing because
	// stdout might have disappeared (due to races with when SIGHUP is sent).
	_ = tty.resize()
	// Handle and forward signals.
	for s := range h.signals {
		switch s {
		case unix.SIGWINCH:
			// Ignore errors resizing, as above.
			_ = tty.resize()
		case unix.SIGCHLD:
			exits, err := h.reap()
			if err != nil {
				logrus.Error(err)
			}
			for _, e := range exits {
				logrus.WithFields(logrus.Fields{
					"pid":    e.pid,
					"status": e.status,
				}).Debug("process exited")
				if e.pid == pid1 {
					// call Wait() on the process even though we already have the exit
					// status because we must ensure that any of the go specific process
					// fun such as flushing pipes are complete before we return.
					process.Wait()
					return nil
				}
			}
		default:
			logrus.Debugf("sending signal to process %s", s)
			if err := unix.Kill(pid1, s.(unix.Signal)); err != nil {
				logrus.Error(err)
			}
		}
	}
	return nil
}

// reap runs wait4 in a loop until we have finished processing any existing exits
// then returns all exits to the main event loop for further processing.
func (h *signalHandler) reap() (exits []exit, err error) {
	var (
		ws  unix.WaitStatus
		rus unix.Rusage
	)
	for {
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, &rus)
		if err != nil {
			if err == unix.ECHILD {
				return exits, nil
			}
			return nil, err
		}
		if pid <= 0 {
			return exits, nil
		}
		exits = append(exits, exit{
			pid:    pid,
			status: utils.ExitStatus(ws),
		})
	}
}

// newProcess returns a new libcontainer Process with the arguments from the
// spec and stdio from the current process.
func newProcess(p specs.Process, init bool, logLevel string) (*libcontainer.Process, error) {
	lp := &libcontainer.Process{
		Args: p.Args,
		Env:  p.Env,
		// TODO: fix libcontainer's API to better support uid/gid in a typesafe way.
		User:            fmt.Sprintf("%d:%d", p.User.UID, p.User.GID),
		Cwd:             p.Cwd,
		Label:           p.SelinuxLabel,
		NoNewPrivileges: &p.NoNewPrivileges,
		AppArmorProfile: p.ApparmorProfile,
		Init:            init,
		LogLevel:        logLevel,
	}

	if p.ConsoleSize != nil {
		lp.ConsoleWidth = uint16(p.ConsoleSize.Width)
		lp.ConsoleHeight = uint16(p.ConsoleSize.Height)
	}

	if p.Capabilities != nil {
		lp.Capabilities = &configs.Capabilities{}
		lp.Capabilities.Bounding = p.Capabilities.Bounding
		lp.Capabilities.Effective = p.Capabilities.Effective
		lp.Capabilities.Inheritable = p.Capabilities.Inheritable
		lp.Capabilities.Permitted = p.Capabilities.Permitted
		lp.Capabilities.Ambient = p.Capabilities.Ambient
	}
	for _, gid := range p.User.AdditionalGids {
		lp.AdditionalGroups = append(lp.AdditionalGroups, strconv.FormatUint(uint64(gid), 10))
	}
	return lp, nil
}

// setupIO modifies the given process config according to the options.
func setupIO(process *libcontainer.Process, rootuid, rootgid int, createTTY, detach bool, sockpath string) (*tty, error) {
	if createTTY {
		process.Stdin = nil
		process.Stdout = nil
		process.Stderr = nil
		t := &tty{}
		if !detach {
			if err := t.initHostConsole(); err != nil {
				return nil, err
			}
			parent, child, err := utils.NewSockPair("console")
			if err != nil {
				return nil, err
			}
			process.ConsoleSocket = child
			t.postStart = append(t.postStart, parent, child)
			go func() {
				t.recvtty(parent)
			}()
		} else {
			// the caller of runc will handle receiving the console master
			conn, err := net.Dial("unix", sockpath)
			if err != nil {
				return nil, err
			}
			uc, ok := conn.(*net.UnixConn)
			if !ok {
				return nil, errors.New("casting to UnixConn failed")
			}
			t.postStart = append(t.postStart, uc)
			socket, err := uc.File()
			if err != nil {
				return nil, err
			}
			t.postStart = append(t.postStart, socket)
			process.ConsoleSocket = socket
		}
		return t, nil
	}
	return setupProcessPipes(process, rootuid, rootgid)
}

func terminate(p *libcontainer.Process) {
	_ = p.Signal(unix.SIGKILL)
	_, _ = p.Wait()
}

func (cli *SandboxCli) run(config *specs.Process, c libcontainer.Container) error {
	var err error
	defer func() {
		if err != nil {
			c.Destroy()
		}
	}()
	process, err := newProcess(*config, true, "info")
	if err != nil {
		return err
	}
	baseFd := 3 + len(process.ExtraFiles)
	for i := baseFd; i < baseFd; i++ {
		_, err = os.Stat(fmt.Sprintf("/proc/self/fd/%d", i))
		if err != nil {
			return errors.WithStack(err)
		}
		process.ExtraFiles = append(process.ExtraFiles, os.NewFile(uintptr(i), "PreserveFD:"+strconv.Itoa(i)))
	}

	rootuid, err := c.Config().HostRootUID()
	if err != nil {
		return errors.WithStack(err)
	}
	rootgid, err := c.Config().HostRootGID()
	if err != nil {
		return errors.WithStack(err)
	}

	handler := newSignalHandler(true)
	tty, err := setupIO(process, rootuid, rootgid, config.Terminal, false, "")
	if err != nil {
		return errors.WithStack(err)
	}
	defer tty.Close()

	err = c.Run(process)

	if err != nil {
		return errors.WithStack(err)
	}

	if err = tty.waitConsole(); err != nil {
		terminate(process)
		return errors.WithStack(err)
	}
	if err = tty.ClosePostStart(); err != nil {
		terminate(process)
		return errors.WithStack(err)
	}

	err = handler.forward(process, tty, false)
	if err != nil {
		terminate(process)
		return errors.WithStack(err)
	}
	/* c.Destroy will send SIGKILL to all process in container */
	if err == nil {
		c.Destroy()
	}
	return err
}
