package plugin

import (
	"errors"
	"fmt"
	driverapi "github.com/docker/go-plugins-helpers/network"
	"github.com/docker/libkv/store"
	"github.com/docker/libnetwork/netlabel"
	"github.com/vishvananda/netlink"
	"strconv"

	. "github.com/xytis/polyp/common"
)

const (
	networkType         = "polyp"
	vethPrefix          = "veth"
	vethLen             = 7
	containerVethPrefix = "eth"
)

type driver struct {
	scope    string
	store    store.Store
	networks networks
}

func NewDriver(scope string, iface string, st store.Store) (driverapi.Driver, error) {
	if li, err := netlink.LinkByName(iface); err != nil {
		return nil, fmt.Errorf("could not find base interface %s, (%v)", iface, err)
	} else {
		driver := &driver{
			scope:    scope,
			store:    st,
			networks: networksNew(li, st),
		}

		return driver, nil
	}
}

func (driver *driver) GetCapabilities() (res *driverapi.CapabilitiesResponse, err error) {
	Log.Debugf("Capabilites request")
	defer func() { Log.Debugf("Capabilites response %v (%v)", res, err) }()
	res = &driverapi.CapabilitiesResponse{
		Scope: driver.scope,
	}
	return
}

func (driver *driver) CreateNetwork(rq *driverapi.CreateNetworkRequest) (err error) {
	Log.Debugf("Create network request %s %+v", rq.NetworkID, rq.Options)
	defer func() { Log.Debugf("Create network response (%v)", err) }()
	Log.Debugf("IPAM datas %v | %v", rq.IPv4Data, rq.IPv6Data)
	var (
		ifname string
		brname string
		number int
		labels map[string]interface{}
		ok     bool
	)
	if labels, ok = rq.Options[netlabel.GenericData].(map[string]interface{}); !ok {
		return ErrMissingParameterMap{}
	}
	if vlan, ok := labels["vlan"].(string); !ok || vlan == "" {
		return ErrMissingParam("vlan")
	} else if number, err = strconv.Atoi(vlan); err != nil {
		return fmt.Errorf("could not parse %s as an integer (%v)", vlan, err)
	}

	if ifname, ok = labels["iface"].(string); !ok || ifname == "" {
		ifname = "vlan" + strconv.Itoa(number)
	}
	if brname, ok = labels["bridge"].(string); !ok || brname == "" {
		brname = "bran" + strconv.Itoa(number)
	}
	config := networkConfig{
		LinkName:   ifname,
		BridgeName: brname,
		Vlan:       number,
		Mtu:        1500, //????
		EnableIPv6: false,
	}
	if err := config.parseIPAM(rq.NetworkID, rq.IPv4Data, rq.IPv6Data); err != nil {
		return err
	}
	if err := config.parseLabels(labels); err != nil {
		return err
	}
	if config.EnableIPv6 {
		Log.Warnf("IPV6 not supported. Go code it yourself!")
	}
	return driver.networks.create(rq.NetworkID, config)
}

func (driver *driver) DeleteNetwork(rq *driverapi.DeleteNetworkRequest) (err error) {
	Log.Debugf("Delete network request %s", rq.NetworkID)
	defer func() { Log.Debugf("Delete network response (%v)", err) }()
	err = driver.networks.delete(rq.NetworkID)
	return
}

func (driver *driver) CreateEndpoint(rq *driverapi.CreateEndpointRequest) (res *driverapi.CreateEndpointResponse, err error) {
	Log.Debugf("Create endpoint request %s:%s", rq.NetworkID, rq.EndpointID)
	defer func() { Log.Debugf("Create endpoint response %v (%v)", res, err) }()
	if rq.Interface == nil {
		err = errors.New("invalid interface info passed")
		return
	}

	// Get the network handler and make sure it exists
	ni, err := driver.networks.get(rq.NetworkID)
	if err != nil {
		return
	}
	if err = ni.endpoints.vacant(rq.EndpointID); err != nil {
		return
	}
	if err = driver.networks.createLink(ni.config); err != nil {
		return
	}

	err = ni.endpoints.create(rq.EndpointID, rq.Interface, ni.config)
	res = &driverapi.CreateEndpointResponse{
		Interface: nil,
	}
	return
}

func (driver *driver) DeleteEndpoint(rq *driverapi.DeleteEndpointRequest) (err error) {
	Log.Debugf("Delete endpoint request %s:%s", rq.NetworkID, rq.EndpointID)
	defer func() { Log.Debugf("Delete endpoint response (%v)", err) }()
	ni, err := driver.networks.get(rq.NetworkID)
	if err != nil {
		return err
	}

	if err = ni.endpoints.delete(rq.EndpointID); err == nil {
		if ni.endpoints.length() == 0 {
			err = driver.networks.deleteLink(ni.config)
		}
	}
	return err
}

func (driver *driver) EndpointInfo(rq *driverapi.InfoRequest) (res *driverapi.InfoResponse, err error) {
	Log.Debugf("Info requested %s:%s", rq.NetworkID, rq.EndpointID)
	defer func() { Log.Debugf("Info response %v (%v)", res, err) }()
	res = &driverapi.InfoResponse{}
	return
}

func (driver *driver) Join(rq *driverapi.JoinRequest) (res *driverapi.JoinResponse, err error) {
	Log.Debugf("Join requested %s:%s, sbox:%s", rq.NetworkID, rq.EndpointID, rq.SandboxKey)
	defer func() { Log.Debugf("Join response %v (%v)", res, err) }()

	ni, err := driver.networks.get(rq.NetworkID)
	if err != nil {
		return
	}

	ep, err := ni.endpoints.get(rq.EndpointID)
	if err != nil {
		return
	}

	res = &driverapi.JoinResponse{
		Gateway:       ni.config.GatewayIPv4.String(),
		InterfaceName: driverapi.InterfaceName{ep.ifname, containerVethPrefix},
	}

	return
}

func (driver *driver) Leave(rq *driverapi.LeaveRequest) (err error) {
	Log.Debugf("Leave requested %s:%s", rq.NetworkID, rq.EndpointID)
	defer func() { Log.Debugf("Leave response (%v)", err) }()
	return err
}

func (driver *driver) DiscoverNew(rq *driverapi.DiscoveryNotification) (err error) {
	Log.Debugf("DiscoverNew requested %d:%v", rq.DiscoveryType, rq.DiscoveryData)
	defer func() { Log.Debugf("DiscoverNew response (%v)", err) }()
	return err
}

func (driver *driver) DiscoverDelete(rq *driverapi.DiscoveryNotification) (err error) {
	Log.Debugf("DiscoverDelete requested %d:%v", rq.DiscoveryType, rq.DiscoveryData)
	defer func() { Log.Debugf("DiscoverDelete response (%v)", err) }()
	return err
}
