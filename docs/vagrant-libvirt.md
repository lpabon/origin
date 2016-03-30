# Create an OpenShift Cluster using Vagrant and libvirt
The following instructions will show how to use Vagrant and libvirt to create
an OpenShift cluster.  The cluster is composed of one master and many minions,
each with a number of 2TB drives used for storage.

Using nodes with storage attached will enable the use of GlusterFS and other
software defined storage systems on OpenShift.

# Host setup
Before creating the cluster, the host must have the following:

* NFS Server running
* Firewall either disabled or allowing NFS communication
* Vagrant, libvirt, qemu-kvm installed

## Memory requirement
Note that each virtual machine defaults to using 3G of RAM.  The default setup
is for one master and one minion, therefore using default the setup will consume
6G of RAM.

On a system with 32G of RAM, I have setup a cluster with one master and 8
minions.

# Install

```
$ git clone -b aplo https://github.com/lpabon/origin.git
$ cd origin
```

# Create default cluster
You need to run the setup as root or as `sudo` because Vagrant must manage the
NFS configuration.

```
# ./aplo/up.sh
```

This will create a cluster composed of one master and a single mininon with
twelve 2TB drives.

Once the cluster is running, type the following to initialize the setup:

```
# vagrant ssh master
$ ./contrib/vagrant/privision-final.sh
```

This will setup a project called `test` and an administrator called `test-admin`.

# Deleting the cluster
To completely wipe the cluster execute the following:

```
# source ./aplo/env.source
# vagrant halt && vagrant destroy -f
# rm -f _output/ _tools/ openshift.local.config/ openshift.local.etcd/
```

# Customizing the cluster
To customize the cluster take adjust the environment variables found
in `./aplo/env.source`.  For example, to create three minions each with eight
2TB drives you can set the following:

```
# source ./aplo/env.source
# export OPENSHIFT_NUM_MINIONS=3
# export OPENSHIFT_NUM_GLUSTERFS_DISKS=8
# vagrant up master && vagrant up
```

>IMPORTANT: Bring up the master first before bringing up the mininons.  The
master not only is provisioned, but also it builds OpenShift binaries used
by the minions.

# More information

* [Application Lifecycle Sample](https://github.com/openshift/origin/blob/master/examples/sample-app/README.md)
