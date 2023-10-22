package remote

import "golang.org/x/crypto/ssh"

type ClientConn struct {
	client *ssh.Client

	server string
	user   string
	pass   string
}
