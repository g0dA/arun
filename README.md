# Usage

```
Usage:  sandbox COMMAND [ARG...] [flags]

Run in a sandbox
Usage:
  sandbox COMMAND [ARG...] [flags]

Flags:
  -c, --config string   Sandbox config path (default "./config")
  -h, --help            help for sandbox
  -u, --user string     User run in Sandbox (default "root")

Example:
  sandbox bash
  sandbox -u testuser bash
  sandbox -c /data/config.json bash
```

# Config
```
Please use json file to set capablities,readonly paths and unmount paths
```
```
{
	"capabilities": {
		"bounding": [
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_SETGID",
			"CAP_SETUID",
			"CAP_SETPCAP",
			"CAP_NET_BIND_SERVICE",
			"CAP_NET_RAW",
			"CAP_SYS_CHROOT",
			"CAP_MKNOD",
			"CAP_AUDIT_WRITE",
			"CAP_SETFCAP"
		],
		"effective": [
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_SETGID",
			"CAP_SETUID",
			"CAP_SETPCAP",
			"CAP_NET_BIND_SERVICE",
			"CAP_NET_RAW",
			"CAP_SYS_CHROOT",
			"CAP_MKNOD",
			"CAP_AUDIT_WRITE",
			"CAP_SETFCAP"
		],
		"inheritable": [
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_SETGID",
			"CAP_SETUID",
			"CAP_SETPCAP",
			"CAP_NET_BIND_SERVICE",
			"CAP_NET_RAW",
			"CAP_SYS_CHROOT",
			"CAP_MKNOD",
			"CAP_AUDIT_WRITE",
			"CAP_SETFCAP"
		],
		"permitted": [
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_SETGID",
			"CAP_SETUID",
			"CAP_SETPCAP",
			"CAP_NET_BIND_SERVICE",
			"CAP_NET_RAW",
			"CAP_SYS_CHROOT",
			"CAP_MKNOD",
			"CAP_AUDIT_WRITE",
			"CAP_SETFCAP"
		],
		"ambient": [
		]
		},

	"readonlyPaths": [
			"/home/lxl/data1",
			"/secret"
		],
	"unmountPaths":[
	        "/mnt"
	]
}



```

# Introduction

`sandbox` is a CLI tool for running commands in a container

# Building

```
go build -o sandbox ./cmd/compass/main.go
```


