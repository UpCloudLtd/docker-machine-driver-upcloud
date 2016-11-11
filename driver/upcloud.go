package upcloud

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/Jalle19/upcloud-go-sdk/upcloud"
	"github.com/Jalle19/upcloud-go-sdk/upcloud/client"
	"github.com/Jalle19/upcloud-go-sdk/upcloud/request"
	"github.com/Jalle19/upcloud-go-sdk/upcloud/service"
	"github.com/docker/machine/libmachine/drivers"
	"github.com/docker/machine/libmachine/log"
	"github.com/docker/machine/libmachine/mcnflag"
	"github.com/docker/machine/libmachine/ssh"
	"github.com/docker/machine/libmachine/state"
)

type Driver struct {
	*drivers.BaseDriver
	User         string
	Passwd       string
	Template     string
	Plan         string
	Zone         string
	ServerUUID   string
	ServerName   string
	UserDataFile string
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
		mcnflag.StringFlag{
			EnvVar: "UPCLOUD_NAME",
			Name:   "upcloud-name",
			Usage:  "a name has to be set ",
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
	d.Plan = flags.String("upcloud-plan")
	d.ServerName = flags.String("upcloud-name")
	d.UserDataFile = flags.String("upcloud-userdata")
	d.SetSwarmConfigFromFlags(flags)

	if d.User == "" || d.Passwd == "" || d.ServerName == "" {
		return fmt.Errorf("upcloud driver requires the --upcloud-user, the --upcloud-passwd, and the --upcloud-name options")
	}

	return nil
}

func (d *Driver) PreCreateCheck() error {
	if d.UserDataFile != "" {
		if _, err := os.Stat(d.UserDataFile); os.IsNotExist(err) {
			return fmt.Errorf("user-data file %s could not be found", d.UserDataFile)
		}
	}

	client := d.getClient()
	service := d.getService(client)
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

	ipAddresses := []request.CreateServerIPAddress{
		{
			Access: upcloud.IPAddressAccessPrivate,
			Family: upcloud.IPAddressFamilyIPv4,
		},
		{
			Access: upcloud.IPAddressAccessPublic,
			Family: upcloud.IPAddressFamilyIPv4,
		},
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

	client := d.getClient()
	service := d.getService(client)

	title := "docker-machine - " + d.ServerName

	createRequest := &request.CreateServerRequest{
		Hostname:       d.ServerName,
		Title:          title,
		Plan:           d.Plan,
		Zone:           d.Zone,
		UserData:       userdata,
		LoginUser:      loginUser,
		StorageDevices: storageDevices,
		IPAddresses:    ipAddresses,
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
			if address.Access == upcloud.IPAddressAccessPublic && address.Family == upcloud.IPAddressFamilyIPv4 {
				d.IPAddress = address.Address
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
	client := d.getClient()
	service := d.getService(client)

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
	client := d.getClient()
	service := d.getService(client)

	request := &request.StartServerRequest{
		UUID: d.ServerUUID,
	}
	_, err := service.StartServer(request)
	return err
}

func (d *Driver) Stop() error {
	client := d.getClient()
	service := d.getService(client)

	request := &request.StopServerRequest{
		UUID: d.ServerUUID,
	}
	_, err := service.StopServer(request)
	return err
}

func (d *Driver) Restart() error {
	client := d.getClient()
	service := d.getService(client)

	request := &request.RestartServerRequest{
		UUID: d.ServerUUID,
	}
	_, err := service.RestartServer(request)
	return err
}

func (d *Driver) Kill() error {
	client := d.getClient()
	service := d.getService(client)

	request := &request.StopServerRequest{
		UUID: d.ServerUUID,
	}
	_, err := service.StopServer(request)
	return err
}

func (d *Driver) Remove() error {
	details, err := d.getServerDetails(d.ServerUUID)

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
	user := d.User
	passwd := d.Passwd

	return client.New(user, passwd)
}

func (d *Driver) getService(client *client.Client) *service.Service {
	return service.New(client)
}

func (d *Driver) getServerDetails(UUID string) (*upcloud.ServerDetails, error) {
	client := d.getClient()
	service := d.getService(client)

	details, err := service.GetServerDetails(&request.GetServerDetailsRequest{UUID: d.ServerUUID})

	return details, err
}

func (d *Driver) deleteStorage(UUID string) error {
	client := d.getClient()
	service := d.getService(client)

	request := &request.DeleteStorageRequest{
		UUID: UUID,
	}

	err := service.DeleteStorage(request)

	return err
}

func (d *Driver) deleteServer(UUID string) error {
	client := d.getClient()
	service := d.getService(client)

	request := &request.DeleteServerRequest{
		UUID: UUID,
	}

	err := service.DeleteServer(request)

	return err
}
