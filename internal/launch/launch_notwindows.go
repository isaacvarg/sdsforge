//go:build !windows

package launch

// The last resort when neither the config file nor the environment names one.
//
// vi and /bin/sh are the two programs POSIX actually requires to be there, so
// these are the only defaults that cannot themselves fail on a bare system.
const (
	defaultEditor = "vi"
	defaultShell  = "/bin/sh"
)
