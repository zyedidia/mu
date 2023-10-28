package remote

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var DefaultTimeout = 10 * time.Second

type Client struct {
	client *ssh.Client
	config Config
}

type Config struct {
	Auth     []ssh.AuthMethod
	User     string
	Addr     string
	Port     uint
	Timeout  time.Duration
	Callback ssh.HostKeyCallback
}

func NewClient(user string, addr string, getpass func() (string, error)) (*Client, error) {
	callback, err := DefaultKnownHosts()
	if err != nil {
		return nil, err
	}

	var auth []ssh.AuthMethod
	sshagent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err == nil {
		auth = append(auth, ssh.PublicKeysCallback(agent.NewClient(sshagent).Signers))
	}
	auth = append(auth, ssh.PasswordCallback(getpass))

	c := &Client{
		config: Config{
			User:     user,
			Addr:     addr,
			Port:     22,
			Auth:     auth,
			Timeout:  DefaultTimeout,
			Callback: callback,
		},
	}

	return c, c.Connect()
}

func (c *Client) Connect() error {
	client, err := ssh.Dial("tcp", net.JoinHostPort(c.config.Addr, fmt.Sprint(c.config.Port)), &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            c.config.Auth,
		Timeout:         c.config.Timeout,
		HostKeyCallback: c.config.Callback,
	})
	c.client = client
	return err
}

func (c *Client) Ssh() *ssh.Client {
	return c.client
}

func (c *Client) String() string {
	return fmt.Sprintf("%s@%s", c.config.User, c.config.Addr)
}

func (c *Client) RunCommand(cmd string) ([]byte, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	sout := &bytes.Buffer{}
	serr := &bytes.Buffer{}
	session.Stdout = sout
	session.Stderr = serr
	if err := session.Run(cmd); err != nil {
		return nil, fmt.Errorf("remote command: %w: %s", err, serr.String())
	}
	return sout.Bytes(), nil
}
