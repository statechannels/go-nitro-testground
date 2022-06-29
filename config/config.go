package config

import (
	"fmt"
)

type RolesConfig struct {
	AmountHubs   uint
	Payees       uint
	Payers       uint
	PayeePayeers uint
	PortStart    int
}

func (c *RolesConfig) Validate(instanceCount uint) error {
	total := c.AmountHubs + c.PayeePayeers + c.Payees + c.Payers
	if total != instanceCount {
		return fmt.Errorf("total number of roles (%d) does not match instance count (%d)", total, instanceCount)
	}
	if c.PortStart <= 0 {
		return fmt.Errorf("port start must be a valid tcp port")
	}
	return nil
}
