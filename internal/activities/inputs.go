package activities

type SubnetConfig struct {
	CIDR             string
	AvailabilityZone string
	Public           bool
}

type CreateVPCInput struct {
	Region      string
	VpcCIDR     string
	Environment string
	Team        string
}

type CreateSubnetsInput struct {
	Region      string
	VpcID       string
	Subnets     []SubnetConfig
	Environment string
	Team        string
}

type CreateInternetGatewayInput struct {
	Region      string
	VpcID       string
	Environment string
	Team        string
}

type ConfigureRouteTablesInput struct {
	Region      string
	VpcID       string
	IgwID       string
	SubnetIDs   []string
	Environment string
	Team        string
}

type CreateEKSClusterInput struct {
	Region      string
	ClusterName string
	RoleARN     string
	VpcID       string
	SubnetIDs   []string
	Environment string
	Team        string
}

type CreateNodeGroupInput struct {
	Region       string
	ClusterName  string
	NodeRoleARN  string
	SubnetIDs    []string
	NodeCount    int32
	InstanceType string
	Environment  string
	Team         string
}

type DeleteNodeGroupInput struct {
	Region      string
	ClusterName string
}

type DeleteEKSClusterInput struct {
	Region      string
	ClusterName string
}

type DeleteSubnetsInput struct {
	Region string
	VpcID  string
}

type DeleteRouteTablesInput struct {
	Region string
	VpcID  string
}

type DetachDeleteInternetGatewayInput struct {
	Region string
	VpcID  string
}

type DeleteVPCInput struct {
	Region string
	VpcID  string
}

type CreateIAMRoleInput struct {
	RoleName    string
	Description string
	TrustPolicy string
	PolicyARNs  []string
	Environment string
	Team        string
}

type DeleteIAMRoleInput struct {
	RoleName string
}
