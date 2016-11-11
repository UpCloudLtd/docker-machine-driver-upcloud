package main

import (
	"github.com/docker/machine/libmachine/drivers/plugin"
	"github.com/torras/docker-machine-driver-upcloud/driver"
)

func main() {
	plugin.RegisterDriver(upcloud.NewDriver("", ""))
}
