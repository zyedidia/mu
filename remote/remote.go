package remote

import (
	"errors"
	"fmt"
	"strings"
)

type Remote struct {
	conns   map[string]*Client
	getpass func() (string, error)
}

func NewRemote(getpass func() (string, error)) *Remote {
	return &Remote{
		conns:   make(map[string]*Client),
		getpass: getpass,
	}
}

func (r *Remote) Connect(name string) (*Client, error) {
	user, addr, found := strings.Cut(name, "@")
	if !found {
		return nil, errors.New("must be of the form user@addr")
	}

	if c, ok := r.conns[name]; ok {
		return c, nil
	}

	c, err := NewClient(user, addr, r.getpass)
	if err != nil {
		return nil, err
	}
	r.conns[name] = c
	return c, nil
}

func (r *Remote) Disconnect(c *Client) {
	r.DisconnectConn(c.config.User, c.config.Addr)
}

func (r *Remote) DisconnectConn(user, addr string) {
	name := fmt.Sprintf("%s@%s", user, addr)
	if c, ok := r.conns[name]; ok {
		c.client.Close()
		delete(r.conns, name)
	}
}
