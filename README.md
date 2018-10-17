# docker-machine-driver-upcloud

A docker-machine driver for [UpCloud](https://www.upcloud.com/)

Originally developed by [Hanzo Studio](https://hanzo.es/)

## How to get it

### The binary way

Download the latest
[binary](https://github.com/torras/docker-machine-driver-upcloud/releases/download/0/docker-machine-driver-upcloud) from the
[releases](https://github.com/torras/docker-machine-driver-upcloud/releases) page,
place it somewhere in your path, like `/usr/local/bin/` or the like and make
sure the binary is executable.

### The go way

_You must first have a working go development environment to get it this way._

```bash
$ go get -u github.com/torras/docker-machine-driver-upcloud
```

### How to use it

_An UpCloud account with api access is needed to use this driver_

Options:

```bash
$ docker-machine create --driver upcloud

...

  --upcloud-user                                             upcloud api access user [$UPCLOUD_USER]
  --upcloud-passwd                                           upcloud api access user's password [$UPCLOUD_PASSWD]
  --upcloud-plan "1xCPU-1GB"                                 upcloud plan [$UPCLOUD_PLAN]
  --upcloud-ssh-user "root"                                  SSH username [$UPCLOUD_SSH_USER]
  --upcloud-template "01000000-0000-4000-8000-000030030200"  upcloud template [$UPCLOUD_TEMPLATE]
  --upcloud-use-private-network                              set this flag to use private networking [$UPCLOUD_USE_PRIVATE_NETWORK]
  --upcloud-use-private-network-only                         set this flag to only use private networking [$UPCLOUD_USE_PRIVATE_NETWORK_ONLY]
  --upcloud-userdata                                         path to file with cloud-init user-data [$UPCLOUD_USERDATA]
  --upcloud-zone "uk-lon1"                                   upcloud zone [$UPCLOUD_ZONE]

...

```

Example run:

```bash
$ docker-machine create \
--driver upcloud \
--upcloud-user "user" \
--upcloud-passwd "password" \
--upcloud-template "an UpCloud's template ID, defaults to Ubuntu Xenial" \
machine_name
```

### Issues, contributions and comments.

Feel free to address me the way you find the most convenient.

Issues, contributions, comments and such are always welcome.
