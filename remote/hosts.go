package remote

import (
	"github.com/zyedidia/mu/pkg/home"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func DefaultKnownHosts() (ssh.HostKeyCallback, error) {
	path, err := home.Expand("~/.ssh/known_hosts")
	if err != nil {
		return nil, err
	}
	return knownhosts.New(path)
}
