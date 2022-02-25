package command

import (
	"io"

	"pdd/sandbox/pkg/stream"

	"github.com/docker/docker/pkg/term"
)

type SandboxCliOption func(cli *SandboxCli) error

type Cli interface {
	Out() *stream.OutStream
	Err() io.Writer
	In() *stream.InStream
	SetIn(in *stream.InStream)
}

// SandboxCli cli struct
type SandboxCli struct {
	in  *stream.InStream
	out *stream.OutStream
	err io.Writer
}

// NewSandboxCli new SandboxCli
func NewSandboxCli(ops ...SandboxCliOption) (*SandboxCli, error) {
	cli := &SandboxCli{}
	if err := cli.Apply(ops...); err != nil {
		return nil, err
	}
	if cli.out == nil || cli.in == nil || cli.err == nil {
		stdin, stdout, stderr := term.StdStreams()
		if cli.in == nil {
			cli.in = stream.NewInStream(stdin)
		}
		if cli.out == nil {
			cli.out = stream.NewOutStream(stdout)
		}
		if cli.err == nil {
			cli.err = stderr
		}
	}
	return cli, nil
}

// Apply all the operation on the cli
func (cli *SandboxCli) Apply(ops ...SandboxCliOption) error {
	for _, op := range ops {
		if err := op(cli); err != nil {
			return err
		}
	}
	return nil
}

// Out returns the writer used for stdout
func (cli *SandboxCli) Out() *stream.OutStream {
	return cli.out
}

// Err returns the writer used for stderr
func (cli *SandboxCli) Err() io.Writer {
	return cli.err
}

// SetIn sets the reader used for stdin
func (cli *SandboxCli) SetIn(in *stream.InStream) {
	cli.in = in
}

// In returns the reader used for stdin
func (cli *SandboxCli) In() *stream.InStream {
	return cli.in
}
