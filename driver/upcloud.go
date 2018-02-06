package upcloud

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/service"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/ssh"
	"github.com/docker/machine/libmachine/state"
)

type Driver struct {
	*drivers.BaseDriver
	User                  string
	Passwd                string
	Template              string
	Plan                  string
	Zone                  string
	UsePrivateNetwork     bool
	UsePrivateNetworkOnly bool
	ServerUUID            string
	ServerName            string
	UserDataFile          string
}

const (
	defaultSSHUser  = "root"
	defaultTemplate = "01000000-0000-4000-8000-000030030200"
	defaultZone     = "uk-lon1"
	defaultPlan     = "1xCPU-1GB"
)

// GetCreateFlags registers the flags this driver adds to
// "docker hosts create"
func (d *Driver) GetCreateFlags() []mcnflag.Flag {
	return []mcnflag.Flag{
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_USER",
			Name:   "upcloud-user",
			Usage:  "upcloud api access user",
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_PASSWD",
			Name:   "upcloud-passwd",
			Usage:  "upcloud api access user's password",
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_SSH_USER",
			Name:   "upcloud-ssh-user",
			Usage:  "SSH username",
			Value:  defaultSSHUser,
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_TEMPLATE",
			Name:   "upcloud-template",
			Usage:  "upcloud template",
			Value:  defaultTemplate,
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_ZONE",
			Name:   "upcloud-zone",
			Usage:  "upcloud zone",
			Value:  defaultZone,
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_PLAN",
			Name:   "upcloud-plan",
			Usage:  "upcloud plan",
			Value:  defaultPlan,
		},
		mcnflag.BoolFlag{
			EnvVar: "UPCLOUD_USE_PRIVATE_NETWORK",
			Name:   "upcloud-use-private-network",
			Usage:  "set this flag to use private networking",
		},
		mcnflag.BoolFlag{
			EnvVar: "UPCLOUD_USE_PRIVATE_NETWORK_ONLY",
			Name:   "upcloud-use-private-network-only",
			Usage:  "set this flag to only use private networking",
		},
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_USERDATA",
			Name:   "upcloud-userdata",
			Usage:  "path to file with cloud-init user-data",
		},
	}
}

func NewDriver(hostName, storePath string) *Driver {
	return &Driver{
		Template: defaultTemplate,
		Plan:     defaultPlan,
		Zone:     defaultZone,
		BaseDriver: &drivers.BaseDriver{
			SSHUser:     defaultSSHUser,
			MachineName: hostName,
			StorePath:   storePath,
		},
	}
}

func (d *Driver) GetSSHHostname() (string, error) {
	return d.GetIP()
}

// DriverName returns the name of the driver
func (d *Driver) DriverName() string {
	return "upcloud"
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	d.User = flags.String("upcloud-user")
	d.Passwd = flags.String("upcloud-passwd")
	d.SSHUser = flags.String("upcloud-ssh-user")
	d.Template = flags.String("upcloud-template")
	d.Zone = flags.String("upcloud-zone")
	d.UsePrivateNetwork = flags.Bool("upcloud-use-private-network")
	d.UsePrivateNetworkOnly = flags.Bool("upcloud-use-private-network-only")
	d.Plan = flags.String("upcloud-plan")
	d.ServerName = d.MachineName
	d.UserDataFile = flags.String("upcloud-userdata")
	d.SetSwarmConfigFromFlags(flags)

	if d.User == "" || d.Passwd == "" {
		return fmt.Errorf("upcloud driver requires upcloud credentials.")
	}

	return nil
}

func (d *Driver) PreCreateCheck() error {
	if d.UserDataFile != "" {
		if _, err := os.Stat(d.UserDataFile); os.IsNotExist(err) {
			return fmt.Errorf("user-data file %s could not be found", d.UserDataFile)
		}
	}

	service := d.getService()
	zones, err := service.GetZones()
	if err != nil {
		return err
	}
	for _, zone := range zones.Zones {
		if zone.Id == d.Zone {
			return nil
		}
	}

	return fmt.Errorf("you should use a valid upcloud zone")
}

func (d *Driver) Create() error {
	var userdata string

	if d.UserDataFile != "" {
		buf, err := ioutil.ReadFile(d.UserDataFile)
		if err != nil {
			return err
		}
		userdata = string(buf)
	}

	log.Infof("creating SSH key")
	if err := ssh.GenerateSSHKey(d.GetSSHKeyPath()); err != nil {
		return err
	}

	publicKey, err := ioutil.ReadFile(d.GetSSHKeyPath() + ".pub")
	if err != nil {
		return err
	}
	strPublicKey := string(publicKey[:])

	loginUser := &request.LoginUser{
		Username: d.SSHUser,
		SSHKeys:  []string{strPublicKey},
	}

	ipAddressesAry := []request.CreateServerIPAddress{
		{
			Access: upcloud.IPAddressAccessPrivate,
			Family: upcloud.IPAddressFamilyIPv4,
		},
	}

	if !d.UsePrivateNetworkOnly {
		ipAddressesAry = append(ipAddressesAry, request.CreateServerIPAddress{
			Access: upcloud.IPAddressAccessPublic,
			Family: upcloud.IPAddressFamilyIPv4,
		})
	}

	storageDevices := []upcloud.CreateServerStorageDevice{
		{
			Action:  upcloud.CreateServerStorageDeviceActionClone,
			Storage: d.Template,
			Title:   "disk1",
			Size:    30,
			Tier:    upcloud.StorageTierMaxIOPS,
		},
	}

	log.Infof("Creating upcloud server...")

	service := d.getService()

	title := "docker-machine - " + d.ServerName

	createRequest := &request.CreateServerRequest{
		Hostname:       d.ServerName,
		Title:          title,
		Plan:           d.Plan,
		Zone:           d.Zone,
		UserData:       userdata,
		LoginUser:      loginUser,
		StorageDevices: storageDevices,
		IPAddresses:    ipAddressesAry,
	}

	newServer, err := service.CreateServer(createRequest)

	if err != nil {
		return err
	}

	d.ServerUUID = newServer.UUID

	getServerDetailsRequest := &request.GetServerDetailsRequest{
		UUID: d.ServerUUID,
	}

	log.Info("Waiting for IP address to be assigned to the server..")
	for {
		newServer, err = service.GetServerDetails(getServerDetailsRequest)
		if err != nil {
			return err
		}
		for _, address := range newServer.IPAddresses {
			if d.UsePrivateNetwork || d.UsePrivateNetworkOnly {
				if address.Access == upcloud.IPAddressAccessPrivate && address.Family == upcloud.IPAddressFamilyIPv4 {
					d.IPAddress = address.Address
				}
			} else {
				if address.Access == upcloud.IPAddressAccessPublic && address.Family == upcloud.IPAddressFamilyIPv4 {
					d.IPAddress = address.Address
				}
			}
		}

		if d.IPAddress != "" {
			break
		}

		time.Sleep(1 * time.Second)
	}

	log.Debugf("Created server with UUID %d and IP address %s",
		d.ServerUUID,
		d.IPAddress)

	return nil
}

func (d *Driver) GetURL() (string, error) {
	if err := drivers.MustBeRunning(d); err != nil {
		return "", err
	}

	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("tcp://%s", net.JoinHostPort(ip, "2376")), nil
}

func (d *Driver) GetState() (state.State, error) {
	service := d.getService()

	getServerDetailsRequest := &request.GetServerDetailsRequest{
		UUID: d.ServerUUID,
	}

	server, err := service.GetServerDetails(getServerDetailsRequest)
	if err != nil {
		return state.Error, err
	}

	st := state.None

	switch server.State {
	case "new":
		st = state.Starting
	case upcloud.ServerStateStarted:
		st = state.Running
	case upcloud.ServerStateStopped:
		st = state.Stopped
	case upcloud.ServerStateError:
		st = state.Error
	}

	return st, nil
}

func (d *Driver) Start() error {
	err := d.startServer(d.ServerUUID)
	return err
}

func (d *Driver) Stop() error {
	err := d.stopServer(d.ServerUUID, request.ServerStopTypeSoft)
	return err
}

func (d *Driver) Restart() error {
	err := d.restartServer(d.ServerUUID)
	return err
}

func (d *Driver) Kill() error {
	err := d.stopServer(d.ServerUUID, request.ServerStopTypeHard)
	return err
}

func (d *Driver) Remove() error {
	details, err := d.getServerDetails(d.ServerUUID)

	err = d.stopServer(details.UUID, request.ServerStopTypeHard)
	if err != nil {
		return err
	}

	err = d.waitForState(upcloud.ServerStateStopped)
	if err != nil {
		return err
	}

	err = d.deleteServer(details.UUID)
	if err != nil {
		return err
	}

	for _, disk := range details.StorageDevices {
		err = d.deleteStorage(disk.UUID)
		if err != nil {
			return err
		}
	}

	return err
}

func (d *Driver) getClient() *client.Client {
	client := client.New(d.User, d.Passwd)
	client.SetTimeout(time.Second * 30)
	return client
}

func (d *Driver) getService() *service.Service {
	return service.New(d.getClient())
}

func (d *Driver) getServerDetails(UUID string) (*upcloud.ServerDetails, error) {
	service := d.getService()

	details, err := service.GetServerDetails(&request.GetServerDetailsRequest{UUID: d.ServerUUID})

	return details, err
}

func (d *Driver) deleteStorage(UUID string) error {
	service := d.getService()

	request := &request.DeleteStorageRequest{
		UUID: UUID,
	}

	err := service.DeleteStorage(request)

	return err
}

func (d *Driver) deleteServer(UUID string) error {
	service := d.getService()

	request := &request.DeleteServerRequest{
		UUID: UUID,
	}

	err := service.DeleteServer(request)

	return err
}

func (d *Driver) startServer(UUID string) error {
	service := d.getService()

	request := &request.StartServerRequest{
		UUID: UUID,
	}

	_, err := service.StartServer(request)

	return err
}

func (d *Driver) stopServer(UUID, stopType string) error {
	service := d.getService()

	request := &request.StopServerRequest{
		UUID:     UUID,
		StopType: stopType,
	}

	_, err := service.StopServer(request)

	return err
}

func (d *Driver) restartServer(UUID string) error {
	service := d.getService()

	request := &request.RestartServerRequest{
		UUID: UUID,
	}

	_, err := service.RestartServer(request)

	return err
}

func (d *Driver) waitForState(state string) error {
	server_state := "unkown"
	var err error

	for server_state != state {
		details, err := d.getServerDetails(d.ServerUUID)

		server_state = details.State

		if err != nil {
			return err
		}
	}

	return err
}
