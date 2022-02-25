package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"

	"github.com/containerd/console"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
)

//tty struct tty
type tty struct {
	epoller   *console.Epoller
	console   *console.EpollConsole
	stdin     console.Console
	closers   []io.Closer
	postStart []io.Closer
	wg        sync.WaitGroup
	current   console.Console
	state     bool
}

func (t *tty) copyIO(w io.Writer, r io.ReadCloser) {
	defer t.wg.Done()
	io.Copy(w, r)
	r.Close()
}

// setup pipes for the process so that advanced features like c/r are able to easily checkpoint
// and restore the process's IO without depending on a host specific path or device
func setupProcessPipes(p *libcontainer.Process, rootuid, rootgid int) (*tty, error) {
	i, err := p.InitializeIO(rootuid, rootgid)
	if err != nil {
		return nil, err
	}
	t := &tty{
		closers: []io.Closer{
			i.Stdin,
			i.Stdout,
			i.Stderr,
		},
	}
	// add the process's io to the post start closers if they support close
	for _, cc := range []interface{}{
		p.Stdin,
		p.Stdout,
		p.Stderr,
	} {
		if c, ok := cc.(io.Closer); ok {
			t.postStart = append(t.postStart, c)
		}
	}
	go func() {
		io.Copy(i.Stdin, os.Stdin)
		i.Stdin.Close()
	}()
	t.wg.Add(2)
	go t.copyIO(os.Stdout, i.Stdout)
	go t.copyIO(os.Stderr, i.Stderr)
	return t, nil
}

func inheritStdio(process *libcontainer.Process) error {
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	return nil
}

func (t *tty) initHostConsole() error {
	// Usually all three (stdin, stdout, and stderr) streams are open to
	// the terminal, but they might be redirected, so try them all.
	for _, s := range []*os.File{os.Stderr, os.Stdout, os.Stdin} {
		c, err := console.ConsoleFromFile(s)
		if err == nil {
			t.stdin = c
			return nil
		}
		if errors.Is(err, console.ErrNotAConsole) {
			continue
		}
		// should not happen
		return fmt.Errorf("unable to get console: %w", err)
	}

	/*
			we found that the stdin of the process which started the sandbox may not point to the tty(for example
		"/dev/null"),so we create a pty for the process.
		    As follows,we use console.NewPty() instead of os.Open("/dev/tty")
	*/

	// If all streams are redirected, but we still have a controlling
	// terminal, it can be obtained by opening /dev/tty.
	//tty, err := os.Open("/dev/tty")
	//if err != nil {
	//	return fmt.Errorf("unable to get tty: %w", err)
	//}
	//

	//c, err := console.ConsoleFromFile(tty)
	c, _, err := console.NewPty()
	if err != nil {
		return err
	}
	if err != nil {
		return fmt.Errorf("unable to get console: %w", err)
	}

	t.stdin = c
	return nil
}

func (t *tty) recvtty(socket *os.File) (Err error) {
	defer func() {
		t.state = true
	}()
	f, err := utils.RecvFd(socket)
	if err != nil {
		return err
	}
	cons, err := console.ConsoleFromFile(f)
	if err != nil {
		return err
	}
	console.ClearONLCR(cons.Fd())
	epoller, err := console.NewEpoller()
	if err != nil {
		return err
	}
	epollConsole, err := epoller.Add(cons)
	if err != nil {
		return err
	}
	defer func() {
		if Err != nil {
			epollConsole.Close()
		}
	}()
	go epoller.Wait()
	go io.Copy(epollConsole, os.Stdin)
	t.wg.Add(1)
	go t.copyIO(os.Stdout, epollConsole)

	if err := t.stdin.SetRaw(); err != nil {
		return fmt.Errorf("failed to set the terminal from the stdin: %v", err)
	}
	go handleInterrupt(t.stdin)

	t.epoller = epoller
	t.console = epollConsole
	t.closers = []io.Closer{epollConsole}
	return nil
}

func handleInterrupt(c console.Console) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan
	c.Reset()
	os.Exit(0)
}

func (t *tty) waitConsole() error {
	return nil
}

// ClosePostStart closes any fds that are provided to the container and dup2'd
// so that we no longer have copy in our process.
func (t *tty) ClosePostStart() error {
	for _, c := range t.postStart {
		c.Close()
	}
	return nil
}

// Close closes all open fds for the tty and/or restores the original
// stdin state to what it was prior to the container execution
func (t *tty) Close() error {
	// ensure that our side of the fds are always closed
	for _, c := range t.postStart {
		c.Close()
	}
	// the process is gone at this point, shutting down the console if we have
	// one and wait for all IO to be finished
	if t.console != nil && t.epoller != nil {
		t.console.Shutdown(t.epoller.CloseConsole)
	}
	if !t.state {
		t.stdin.Close()
		return nil
	}
	t.wg.Wait()
	for _, c := range t.closers {
		c.Close()
	}
	if t.stdin != nil {
		t.stdin.Reset()
	}
	return nil
}

func (t *tty) resize() error {
	if t.console == nil {
		return nil
	}
	return t.console.ResizeFrom(t.stdin)
}
