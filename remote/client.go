package remote

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
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

func NewClient(user string, addr string, auth []ssh.AuthMethod) (*Client, error) {
	callback, err := DefaultKnownHosts()
	if err != nil {
		return nil, err
	}

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
