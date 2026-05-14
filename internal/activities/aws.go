package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"go.temporal.io/sdk/activity"
)

type AWSActivities struct {
	roleARN string
}

func NewAWSActivities(roleARN string) *AWSActivities {
	return &AWSActivities{roleARN: roleARN}
}

func (a *AWSActivities) loadConfig(ctx context.Context, region string) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return aws.Config{}, err
	}
	if a.roleARN == "" {
		return cfg, nil
	}
	stsSvc := sts.NewFromConfig(cfg)
	cfg.Credentials = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(stsSvc, a.roleARN))
	return cfg, nil
}

func ec2TagSpec(
	resourceType ec2types.ResourceType,
	environment, team string,
) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{{
		ResourceType: resourceType,
		Tags: []ec2types.Tag{
			{Key: aws.String("ManagedBy"), Value: aws.String("temporal")},
			{Key: aws.String("Environment"), Value: aws.String(environment)},
			{Key: aws.String("Team"), Value: aws.String(team)},
		},
	}}
}

func eksTags(environment, team string) map[string]string {
	return map[string]string{
		"ManagedBy":   "temporal",
		"Environment": environment,
		"Team":        team,
	}
}

func newEC2Client(ctx context.Context, a *AWSActivities, region string) (*ec2.Client, error) {
	cfg, err := a.loadConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}

func newEKSClient(ctx context.Context, a *AWSActivities, region string) (*eks.Client, error) {
	cfg, err := a.loadConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return eks.NewFromConfig(cfg), nil
}

// heartbeatWhileWaiting runs wait in a goroutine and records a heartbeat every
// 10 s so Temporal can detect stalls and propagate cancellations.
func heartbeatWhileWaiting(ctx context.Context, wait func() error) error {
	done := make(chan error, 1)
	go func() { done <- wait() }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			activity.RecordHeartbeat(ctx, nil)
		}
	}
}

func (a *AWSActivities) CreateVPC(ctx context.Context, input CreateVPCInput) (string, error) {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return "", err
	}

	cidr := input.VpcCIDR
	if cidr == "" {
		cidr = "10.0.0.0/16"
	}

	out, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock:         aws.String(cidr),
		TagSpecifications: ec2TagSpec(ec2types.ResourceTypeVpc, input.Environment, input.Team),
	})
	if err != nil {
		return "", fmt.Errorf("create VPC: %w", err)
	}

	vpcID := out.Vpc.VpcId
	if _, err = client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:            vpcID,
		EnableDnsSupport: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
	}); err != nil {
		return "", fmt.Errorf("enable DNS support: %w", err)
	}
	if _, err = client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
		VpcId:              vpcID,
		EnableDnsHostnames: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
	}); err != nil {
		return "", fmt.Errorf("enable DNS hostnames: %w", err)
	}

	return *vpcID, nil
}

func (a *AWSActivities) CreateSubnets(
	ctx context.Context,
	input CreateSubnetsInput,
) ([]string, error) {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return nil, err
	}

	subnets := input.Subnets
	if len(subnets) == 0 {
		subnets = []SubnetConfig{
			{CIDR: "10.0.1.0/24"},
			{CIDR: "10.0.2.0/24"},
		}
	}

	var subnetIDs []string
	for _, sc := range subnets {
		req := &ec2.CreateSubnetInput{
			VpcId:     aws.String(input.VpcID),
			CidrBlock: aws.String(sc.CIDR),
			TagSpecifications: ec2TagSpec(
				ec2types.ResourceTypeSubnet,
				input.Environment,
				input.Team,
			),
		}
		if sc.AvailabilityZone != "" {
			req.AvailabilityZone = aws.String(sc.AvailabilityZone)
		}
		out, err := client.CreateSubnet(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("create subnet %s: %w", sc.CIDR, err)
		}
		subnetID := out.Subnet.SubnetId
		if sc.Public {
			if _, err = client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
				SubnetId:            subnetID,
				MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
			}); err != nil {
				return nil, fmt.Errorf("enable public IP for subnet %s: %w", sc.CIDR, err)
			}
		}
		subnetIDs = append(subnetIDs, *subnetID)
	}

	return subnetIDs, nil
}

func (a *AWSActivities) CreateInternetGateway(
	ctx context.Context,
	input CreateInternetGatewayInput,
) (string, error) {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return "", err
	}

	igwOut, err := client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: ec2TagSpec(
			ec2types.ResourceTypeInternetGateway,
			input.Environment,
			input.Team,
		),
	})
	if err != nil {
		return "", fmt.Errorf("create IGW: %w", err)
	}

	igwID := igwOut.InternetGateway.InternetGatewayId
	if _, err = client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: igwID,
		VpcId:             aws.String(input.VpcID),
	}); err != nil {
		return "", fmt.Errorf("attach IGW: %w", err)
	}

	return *igwID, nil
}

func (a *AWSActivities) ConfigureRouteTables(
	ctx context.Context,
	input ConfigureRouteTablesInput,
) error {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return err
	}

	rtOut, err := client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId: aws.String(input.VpcID),
		TagSpecifications: ec2TagSpec(
			ec2types.ResourceTypeRouteTable,
			input.Environment,
			input.Team,
		),
	})
	if err != nil {
		return fmt.Errorf("create route table: %w", err)
	}
	rtID := *rtOut.RouteTable.RouteTableId

	_, err = client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(rtID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(input.IgwID),
	})
	if err != nil {
		return fmt.Errorf("create default route: %w", err)
	}

	for _, subnetID := range input.SubnetIDs {
		if _, err = client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(rtID),
			SubnetId:     aws.String(subnetID),
		}); err != nil {
			return fmt.Errorf("associate route table to %s: %w", subnetID, err)
		}
	}

	return nil
}

func (a *AWSActivities) CreateEKSCluster(ctx context.Context, input CreateEKSClusterInput) error {
	client, err := newEKSClient(ctx, a, input.Region)
	if err != nil {
		return err
	}

	_, err = client.CreateCluster(ctx, &eks.CreateClusterInput{
		Name: aws.String(input.ClusterName),
		ResourcesVpcConfig: &ekstypes.VpcConfigRequest{
			SubnetIds: input.SubnetIDs,
		},
		RoleArn: aws.String(input.RoleARN),
		Tags:    eksTags(input.Environment, input.Team),
	})
	if err != nil {
		return fmt.Errorf("create EKS cluster: %w", err)
	}

	waiter := eks.NewClusterActiveWaiter(client)
	return heartbeatWhileWaiting(ctx, func() error {
		return waiter.Wait(
			ctx,
			&eks.DescribeClusterInput{Name: aws.String(input.ClusterName)},
			30*time.Minute,
		)
	})
}

func (a *AWSActivities) CreateNodeGroup(ctx context.Context, input CreateNodeGroupInput) error {
	client, err := newEKSClient(ctx, a, input.Region)
	if err != nil {
		return err
	}

	_, err = client.CreateNodegroup(ctx, &eks.CreateNodegroupInput{
		ClusterName:   aws.String(input.ClusterName),
		NodegroupName: aws.String(input.ClusterName + "-nodes"),
		Subnets:       input.SubnetIDs,
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			DesiredSize: aws.Int32(input.NodeCount),
			MinSize:     aws.Int32(1),
			MaxSize:     aws.Int32(input.NodeCount * 2),
		},
		InstanceTypes: []string{input.InstanceType},
		NodeRole:      aws.String(input.NodeRoleARN),
		Tags:          eksTags(input.Environment, input.Team),
	})
	if err != nil {
		return fmt.Errorf("create node group: %w", err)
	}

	waiter := eks.NewNodegroupActiveWaiter(client)
	return heartbeatWhileWaiting(ctx, func() error {
		return waiter.Wait(ctx, &eks.DescribeNodegroupInput{
			ClusterName:   aws.String(input.ClusterName),
			NodegroupName: aws.String(input.ClusterName + "-nodes"),
		}, 30*time.Minute)
	})
}

func (a *AWSActivities) DeleteNodeGroup(ctx context.Context, input DeleteNodeGroupInput) error {
	client, err := newEKSClient(ctx, a, input.Region)
	if err != nil {
		return err
	}

	_, err = client.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
		ClusterName:   aws.String(input.ClusterName),
		NodegroupName: aws.String(input.ClusterName + "-nodes"),
	})
	if err != nil {
		return fmt.Errorf("delete node group: %w", err)
	}

	waiter := eks.NewNodegroupDeletedWaiter(client)
	return heartbeatWhileWaiting(ctx, func() error {
		return waiter.Wait(ctx, &eks.DescribeNodegroupInput{
			ClusterName:   aws.String(input.ClusterName),
			NodegroupName: aws.String(input.ClusterName + "-nodes"),
		}, 30*time.Minute)
	})
}

func (a *AWSActivities) DeleteEKSCluster(ctx context.Context, input DeleteEKSClusterInput) error {
	client, err := newEKSClient(ctx, a, input.Region)
	if err != nil {
		return err
	}

	_, err = client.DeleteCluster(ctx, &eks.DeleteClusterInput{Name: aws.String(input.ClusterName)})
	if err != nil {
		return fmt.Errorf("delete EKS cluster: %w", err)
	}

	waiter := eks.NewClusterDeletedWaiter(client)
	return heartbeatWhileWaiting(ctx, func() error {
		return waiter.Wait(
			ctx,
			&eks.DescribeClusterInput{Name: aws.String(input.ClusterName)},
			30*time.Minute,
		)
	})
}

func (a *AWSActivities) DeleteSubnets(ctx context.Context, input DeleteSubnetsInput) error {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return err
	}

	subnets, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{input.VpcID}},
		},
	})
	if err != nil {
		return fmt.Errorf("describe subnets: %w", err)
	}

	for _, subnet := range subnets.Subnets {
		if _, err = client.DeleteSubnet(
			ctx,
			&ec2.DeleteSubnetInput{SubnetId: subnet.SubnetId},
		); err != nil {
			return fmt.Errorf("delete subnet %s: %w", *subnet.SubnetId, err)
		}
	}

	return nil
}

func (a *AWSActivities) DeleteRouteTables(ctx context.Context, input DeleteRouteTablesInput) error {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return err
	}

	rts, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{input.VpcID}},
		},
	})
	if err != nil {
		return fmt.Errorf("describe route tables: %w", err)
	}

	for _, rt := range rts.RouteTables {
		isMain := false
		for _, assoc := range rt.Associations {
			if aws.ToBool(assoc.Main) {
				isMain = true
				break
			}
		}
		if isMain {
			continue
		}
		if _, err = client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: rt.RouteTableId,
		}); err != nil {
			return fmt.Errorf("delete route table %s: %w", *rt.RouteTableId, err)
		}
	}

	return nil
}

func (a *AWSActivities) DetachDeleteInternetGateway(
	ctx context.Context,
	input DetachDeleteInternetGatewayInput,
) error {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return err
	}

	igws, err := client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("attachment.vpc-id"), Values: []string{input.VpcID}},
		},
	})
	if err != nil || len(igws.InternetGateways) == 0 {
		return err
	}

	igwID := igws.InternetGateways[0].InternetGatewayId
	if _, err = client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
		InternetGatewayId: igwID,
		VpcId:             aws.String(input.VpcID),
	}); err != nil {
		return fmt.Errorf("detach IGW: %w", err)
	}

	_, err = client.DeleteInternetGateway(
		ctx,
		&ec2.DeleteInternetGatewayInput{InternetGatewayId: igwID},
	)
	return err
}

func (a *AWSActivities) DeleteVPC(ctx context.Context, input DeleteVPCInput) error {
	client, err := newEC2Client(ctx, a, input.Region)
	if err != nil {
		return err
	}

	_, err = client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(input.VpcID)})
	return err
}
