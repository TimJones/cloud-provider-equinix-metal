package metal

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/packethost/packngo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cpapi "k8s.io/cloud-provider/api"
)

var projectID = uuid.New().String()

// testNode provides a simple Node object satisfying the lookup requirements of InstanceMetadata()
func testNodeWithIP(providerID, nodeName, nodeIP string) *v1.Node {
	node := testNode(providerID, nodeName)
	if nodeIP != "" {
		node.Annotations = map[string]string{
			cpapi.AnnotationAlphaProvidedIPAddr: nodeIP,
		}
	}
	return node
}

func testNode(providerID, nodeName string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Spec: v1.NodeSpec{
			ProviderID: providerID,
		},
	}
}

func TestNodeAddresses(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	if inst == nil {
		t.Fatal("inst is nil")
	}
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)
	// update the addresses on the device; normally created by Equinix Metal itself
	networks := []*packngo.IPAddressAssignment{
		testCreateAddress(false, false), // private ipv4
		testCreateAddress(false, true),  // public ipv4
		testCreateAddress(true, true),   // public ipv6
	}
	kubeletNodeIP := testCreateAddress(false, false)
	dev.Network = networks
	err := backend.UpdateDevice(dev.ID, dev)
	if err != nil {
		t.Fatalf("unable to update inactive device: %v", err)
	}

	validAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: devName},
		{Type: v1.NodeInternalIP, Address: networks[0].Address},
		{Type: v1.NodeExternalIP, Address: networks[1].Address},
	}

	validAddressesWithProvidedIP := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: devName},
		{Type: v1.NodeInternalIP, Address: kubeletNodeIP.Address},
		{Type: v1.NodeInternalIP, Address: networks[0].Address},
		{Type: v1.NodeExternalIP, Address: networks[1].Address},
	}

	tests := []struct {
		testName  string
		node      *v1.Node
		addresses []v1.NodeAddress
		err       error
	}{
		{"empty node name", testNode("", ""), nil, fmt.Errorf("node name cannot be empty")},
		{"instance not found", testNode("", nodeName), nil, fmt.Errorf("instance not found")},
		{"invalid id", testNode("equinixmetal://123", nodeName), nil, fmt.Errorf("123 is not a valid UUID")},
		{"unknown name", testNode("equinixmetal://"+randomID, nodeName), nil, fmt.Errorf("instance not found")},
		{"valid both", testNode("equinixmetal://"+dev.ID, devName), validAddresses, nil},
		{"valid provider id", testNode("equinixmetal://"+dev.ID, nodeName), validAddresses, nil},
		{"valid node name", testNode("", devName), validAddresses, nil},
		{"with node IP", testNodeWithIP("equinixmetal://"+dev.ID, nodeName, kubeletNodeIP.Address), validAddressesWithProvidedIP, nil},
	}

	for i, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var addresses []v1.NodeAddress

			md, err := inst.InstanceMetadata(context.TODO(), tt.node)
			if md != nil {
				addresses = md.NodeAddresses
			}
			switch {
			case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
				t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
			case !compareAddresses(addresses, tt.addresses):
				t.Errorf("%d: mismatched addresses, actual %v expected %v", i, addresses, tt.addresses)
			}
		})
	}
}

func TestNodeAddressesByProviderID(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)
	// update the addresses on the device; normally created by Equinix Metal itself
	networks := []*packngo.IPAddressAssignment{
		testCreateAddress(false, false), // private ipv4
		testCreateAddress(false, true),  // public ipv4
		testCreateAddress(true, true),   // public ipv6
	}
	dev.Network = networks
	err := backend.UpdateDevice(dev.ID, dev)
	if err != nil {
		t.Fatalf("unable to update inactive device: %v", err)
	}

	validAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: devName},
		{Type: v1.NodeInternalIP, Address: networks[0].Address},
		{Type: v1.NodeExternalIP, Address: networks[1].Address},
	}

	tests := []struct {
		testName  string
		id        string
		addresses []v1.NodeAddress
		err       error
	}{
		{"empty ID", "", nil, fmt.Errorf("instance not found")},
		{"invalid format", randomID, nil, fmt.Errorf("instance not found")},
		{"not equinixmetal", "aws://" + randomID, nil, fmt.Errorf("provider name from providerID should be equinixmetal")},
		{"unknown ID", "equinixmetal://" + randomID, nil, fmt.Errorf("instance not found")},
		{"valid prefix", fmt.Sprintf("equinixmetal://%s", dev.ID), validAddresses, nil},
		{"valid legacy prefix", fmt.Sprintf("packet://%s", dev.ID), validAddresses, nil},
		{"valid without prefix", dev.ID, validAddresses, nil},
	}

	for i, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var addresses []v1.NodeAddress

			md, err := inst.InstanceMetadata(context.TODO(), testNode(tt.id, nodeName))
			if md != nil {
				addresses = md.NodeAddresses
			}
			switch {
			case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
				t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
			case !compareAddresses(addresses, tt.addresses):
				t.Errorf("%d: mismatched addresses, actual %v expected %v", i, addresses, tt.addresses)
			}
		})
	}
}

/*
	func TestInstanceID(t *testing.T) {
		vc, backend := testGetValidCloud(t, "")
		inst, _ := vc.InstancesV2()
		devName := testGetNewDevName()
		facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
		plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
		dev, _ := backend.CreateDevice(projectID, devName, plan, facility)

		tests := []struct {
			name string
			id   string
			err  error
		}{
			{"", "", fmt.Errorf("node name cannot be empty")},          // empty name
			{"thisdoesnotexist", "", fmt.Errorf("instance not found")}, // unknown name
			{devName, dev.ID, nil},                                     // valid
		}

		for i, tt := range tests {
			var id string
			md, err := inst.InstanceMetadata(context.TODO(), testNode(tt.id, nodeName))
			if md != nil {
				id, err = deviceIDFromProviderID(md.ProviderID)
			}

			switch {
			case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
				t.Errorf("%d: mismatched errors, actual %q expected %q", i, err, tt.err)
			case id != tt.id:
				t.Errorf("%d: mismatched id, actual %v expected %v", i, id, tt.id)
			}
		}
	}
*/
func TestInstanceType(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)
	privateIP := "10.1.1.2"
	publicIP := "25.50.75.100"
	dev.Network = append(dev.Network, []*packngo.IPAddressAssignment{
		{IpAddressCommon: packngo.IpAddressCommon{Address: privateIP, Management: true, AddressFamily: 4}},
		{IpAddressCommon: packngo.IpAddressCommon{Address: publicIP, Public: true, AddressFamily: 4}},
	}...)

	tests := []struct {
		testName string
		name     string
		plan     string
		err      error
	}{
		{"empty name", "", "", fmt.Errorf("instance not found")},
		{"invalid id", "thisdoesnotexist", "", fmt.Errorf("thisdoesnotexist is not a valid UUID")},
		{"unknown name", randomID, "", fmt.Errorf("instance not found")},
		{"valid", "equinixmetal://" + dev.ID, dev.Plan.Slug, nil},
	}

	for i, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var plan string
			md, err := inst.InstanceMetadata(context.TODO(), testNode(tt.name, nodeName))
			if md != nil {
				plan = md.InstanceType
			}
			switch {
			case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
				t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
			case plan != tt.plan:
				t.Errorf("%d: mismatched id, actual %v expected %v", i, plan, tt.plan)
			}
		})
	}
}

func TestInstanceZone(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)
	privateIP := "10.1.1.2"
	publicIP := "25.50.75.100"
	metro := &packngo.Metro{ID: "123", Code: validRegionCode, Name: validRegionName, Country: "Country"}
	dev.Metro = metro
	facility.Metro = metro
	dev.Network = append(dev.Network, []*packngo.IPAddressAssignment{
		{IpAddressCommon: packngo.IpAddressCommon{Address: privateIP, Management: true, AddressFamily: 4}},
		{IpAddressCommon: packngo.IpAddressCommon{Address: publicIP, Public: true, AddressFamily: 4}},
	}...)

	tests := []struct {
		testName string
		name     string
		region   string
		zone     string
		err      error
	}{
		{"empty name", "", "", "", fmt.Errorf("instance not found")},
		{"invalid id", "thisdoesnotexist", "", "", fmt.Errorf("thisdoesnotexist is not a valid UUID")},
		{"unknown name", randomID, "", "", fmt.Errorf("instance not found")},
		{"valid", "equinixmetal://" + dev.ID, validRegionCode, validZoneCode, nil},
	}

	for i, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var zone, region string
			md, err := inst.InstanceMetadata(context.TODO(), testNode(tt.name, nodeName))
			if md != nil {
				zone = md.Zone
				region = md.Region
			}
			switch {
			case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
				t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
			case zone != tt.zone:
				t.Errorf("%d: mismatched zone, actual %v expected %v", i, zone, tt.zone)
			case region != tt.region:
				t.Errorf("%d: mismatched region, actual %v expected %v", i, region, tt.region)
			}
		})
	}
}

/*
func TestInstanceTypeByProviderID(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.Instances()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)

	tests := []struct {
		id   string
		plan string
		err  error
	}{
		{"", "", fmt.Errorf("providerID cannot be empty")},                                            // empty name
		{randomID, "", fmt.Errorf("instance not found")},                                              // invalid format
		{"aws://" + randomID, "", fmt.Errorf("provider name from providerID should be equinixmetal")}, // not equinixmetalk
		{"equinixmetal://" + randomID, "", fmt.Errorf("instance not found")},                          // unknown ID
		{fmt.Sprintf("equinixmetal://%s", dev.ID), dev.Plan.Name, nil},                                // valid
		{fmt.Sprintf("packet://%s", dev.ID), dev.Plan.Name, nil},                                      // valid
	}

	for i, tt := range tests {
		plan, err := inst.InstanceTypeByProviderID(nil, tt.id)
		switch {
		case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
			t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
		case plan != tt.plan:
			t.Errorf("%d: mismatched id, actual %v expected %v", i, plan, tt.plan)
		}
	}
}

func TestAddSSHKeyToAllInstances(t *testing.T) {
	vc, _ := testGetValidCloud(t, "")
	inst, _ := vc.Instances()
	err := inst.AddSSHKeyToAllInstances(nil, "", nil)
	if err != cloudprovider.NotImplemented {
		t.Errorf("mismatched error: expected %v received %v", cloudprovider.NotImplemented, err)
	}
}

func TestCurrentNodeName(t *testing.T) {
	vc, _ := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	var (
		devName       = testGetNewDevName()
		expectedError error
		expectedName  = types.NodeName(devName)
	)

	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)

	md, err := inst.InstanceMetadata(context.TODO(), testNode("equinixmetal://"+dev.ID, nodeName))

	if err != expectedError {
		t.Errorf("mismatched errors, actual %v expected %v", err, expectedError)
	}
	if md. != expectedName {
		t.Errorf("mismatched nodename, actual %v expected %v", nn, expectedName)
	}
}

*/

func TestInstanceExistsByProviderID(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	dev, _ := backend.CreateDevice(projectID, devName, plan, facility)

	tests := []struct {
		id     string
		exists bool
		err    error
	}{
		{"", false, fmt.Errorf("providerID cannot be empty")}, // empty name
		{randomID, false, nil},                                // invalid format
		{"aws://" + randomID, false, fmt.Errorf("provider name from providerID should be equinixmetal")}, // not equinixmetal
		{"equinixmetal://" + randomID, false, nil},                                                       // unknown ID
		{fmt.Sprintf("equinixmetal://%s", dev.ID), true, nil},                                            // valid
		{fmt.Sprintf("packet://%s", dev.ID), true, nil},                                                  // valid
		{dev.ID, true, nil}, // valid
	}

	for i, tt := range tests {
		exists, err := inst.InstanceExists(context.TODO(), testNode(tt.id, nodeName))
		switch {
		case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
			t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
		case exists != tt.exists:
			t.Errorf("%d: mismatched exists, actual %v expected %v", i, exists, tt.exists)
		}
	}
}

func TestInstanceShutdownByProviderID(t *testing.T) {
	vc, backend := testGetValidCloud(t, "")
	inst, _ := vc.InstancesV2()
	devName := testGetNewDevName()
	facility, _ := testGetOrCreateValidZone(validZoneName, validZoneCode, backend)
	plan, _ := testGetOrCreateValidPlan(validPlanName, validPlanSlug, backend)
	devActive, _ := backend.CreateDevice(projectID, devName, plan, facility)
	devInactive, _ := backend.CreateDevice(projectID, devName, plan, facility)
	devInactive.State = "inactive"
	err := backend.UpdateDevice(devInactive.ID, devInactive)
	if err != nil {
		t.Fatalf("unable to update inactive device: %v", err)
	}

	tests := []struct {
		id   string
		down bool
		err  error
	}{
		{"", false, fmt.Errorf("providerID cannot be empty")},                                            // empty name
		{randomID, false, fmt.Errorf("instance not found")},                                              // invalid format
		{"aws://" + randomID, false, fmt.Errorf("provider name from providerID should be equinixmetal")}, // not equinixmetal
		{"equinixmetal://" + randomID, false, fmt.Errorf("instance not found")},                          // unknown ID
		{fmt.Sprintf("equinixmetal://%s", devActive.ID), false, nil},                                     // valid
		{fmt.Sprintf("packet://%s", devActive.ID), false, nil},                                           // valid
		{devActive.ID, false, nil},                                                                       // valid
		{fmt.Sprintf("equinixmetal://%s", devInactive.ID), true, nil},                                    // valid
		{fmt.Sprintf("packet://%s", devInactive.ID), true, nil},                                          // valid
		{devInactive.ID, true, nil},                                                                      // valid
	}

	for i, tt := range tests {
		down, err := inst.InstanceShutdown(context.TODO(), testNode(tt.id, nodeName))
		switch {
		case (err == nil && tt.err != nil) || (err != nil && tt.err == nil) || (err != nil && tt.err != nil && !strings.HasPrefix(err.Error(), tt.err.Error())):
			t.Errorf("%d: mismatched errors, actual %v expected %v", i, err, tt.err)
		case down != tt.down:
			t.Errorf("%d: mismatched down, actual %v expected %v", i, down, tt.down)
		}
	}
}

func compareAddresses(a1, a2 []v1.NodeAddress) bool {
	switch {
	case (a1 == nil && a2 != nil) || (a1 != nil && a2 == nil):
		return false
	case a1 == nil && a2 == nil:
		return true
	case len(a1) != len(a2):
		return false
	default:
		for i := range a1 {
			if a1[i] != a2[i] {
				return false
			}
		}
		return true
	}
}
