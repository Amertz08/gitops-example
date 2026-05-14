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

func ec2TagSpec(resourceType ec2types.ResourceType, environment, team string) []ec2types.TagSpecification {
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

func (a *AWSActivities) CreateVPC(ctx context.Context, region, environment, team string) (string, error) {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return "", err
	}

	out, err := client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock:         aws.String("10.0.0.0/16"),
		TagSpecifications: ec2TagSpec(ec2types.ResourceTypeVpc, environment, team),
	})
	if err != nil {
		return "", fmt.Errorf("create VPC: %w", err)
	}

	return *out.Vpc.VpcId, nil
}

func (a *AWSActivities) CreateSubnets(ctx context.Context, region, vpcID, environment, team string) ([]string, error) {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return nil, err
	}

	cidrs := []string{"10.0.1.0/24", "10.0.2.0/24"}
	var subnetIDs []string
	for _, cidr := range cidrs {
		out, err := client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:             aws.String(vpcID),
			CidrBlock:         aws.String(cidr),
			TagSpecifications: ec2TagSpec(ec2types.ResourceTypeSubnet, environment, team),
		})
		if err != nil {
			return nil, fmt.Errorf("create subnet %s: %w", cidr, err)
		}
		subnetIDs = append(subnetIDs, *out.Subnet.SubnetId)
	}

	return subnetIDs, nil
}

func (a *AWSActivities) CreateInternetGateway(ctx context.Context, region, vpcID, environment, team string) (string, error) {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return "", err
	}

	igwOut, err := client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: ec2TagSpec(ec2types.ResourceTypeInternetGateway, environment, team),
	})
	if err != nil {
		return "", fmt.Errorf("create IGW: %w", err)
	}

	igwID := igwOut.InternetGateway.InternetGatewayId
	if _, err = client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: igwID,
		VpcId:             aws.String(vpcID),
	}); err != nil {
		return "", fmt.Errorf("attach IGW: %w", err)
	}

	return *igwID, nil
}

func (a *AWSActivities) ConfigureRouteTables(
	ctx context.Context,
	region, vpcID, igwID string,
	subnetIDs []string,
	environment, team string,
) error {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return err
	}

	rtOut, err := client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{
		VpcId:             aws.String(vpcID),
		TagSpecifications: ec2TagSpec(ec2types.ResourceTypeRouteTable, environment, team),
	})
	if err != nil {
		return fmt.Errorf("create route table: %w", err)
	}
	rtID := *rtOut.RouteTable.RouteTableId

	_, err = client.CreateRoute(ctx, &ec2.CreateRouteInput{
		RouteTableId:         aws.String(rtID),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
	})
	if err != nil {
		return fmt.Errorf("create default route: %w", err)
	}

	for _, subnetID := range subnetIDs {
		if _, err = client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(rtID),
			SubnetId:     aws.String(subnetID),
		}); err != nil {
			return fmt.Errorf("associate route table to %s: %w", subnetID, err)
		}
	}

	return nil
}

func (a *AWSActivities) CreateEKSCluster(
	ctx context.Context,
	region, clusterName, vpcID string,
	subnetIDs []string,
	environment, team string,
) error {
	client, err := newEKSClient(ctx, a, region)
	if err != nil {
		return err
	}

	_, err = client.CreateCluster(ctx, &eks.CreateClusterInput{
		Name: aws.String(clusterName),
		ResourcesVpcConfig: &ekstypes.VpcConfigRequest{
			SubnetIds: subnetIDs,
		},
		RoleArn: aws.String(fmt.Sprintf("arn:aws:iam::*:role/%s-eks-role", clusterName)),
		Tags:    eksTags(environment, team),
	})
	if err != nil {
		return fmt.Errorf("create EKS cluster: %w", err)
	}

	waiter := eks.NewClusterActiveWaiter(client)
	return waiter.Wait(
		ctx,
		&eks.DescribeClusterInput{Name: aws.String(clusterName)},
		30*time.Minute,
	)
}

func (a *AWSActivities) CreateNodeGroup(
	ctx context.Context,
	region, clusterName string,
	subnetIDs []string,
	nodeCount int32,
	instanceType, environment, team string,
) error {
	client, err := newEKSClient(ctx, a, region)
	if err != nil {
		return err
	}

	_, err = client.CreateNodegroup(ctx, &eks.CreateNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(clusterName + "-nodes"),
		Subnets:       subnetIDs,
		ScalingConfig: &ekstypes.NodegroupScalingConfig{
			DesiredSize: aws.Int32(nodeCount),
			MinSize:     aws.Int32(1),
			MaxSize:     aws.Int32(nodeCount * 2),
		},
		InstanceTypes: []string{instanceType},
		NodeRole:      aws.String(fmt.Sprintf("arn:aws:iam::*:role/%s-node-role", clusterName)),
		Tags:          eksTags(environment, team),
	})
	return err
}

func (a *AWSActivities) DeleteNodeGroup(ctx context.Context, region, clusterName string) error {
	client, err := newEKSClient(ctx, a, region)
	if err != nil {
		return err
	}

	_, err = client.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(clusterName + "-nodes"),
	})
	if err != nil {
		return fmt.Errorf("delete node group: %w", err)
	}

	waiter := eks.NewNodegroupDeletedWaiter(client)
	return waiter.Wait(ctx, &eks.DescribeNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(clusterName + "-nodes"),
	}, 30*time.Minute)
}

func (a *AWSActivities) DeleteEKSCluster(ctx context.Context, region, clusterName string) error {
	client, err := newEKSClient(ctx, a, region)
	if err != nil {
		return err
	}

	_, err = client.DeleteCluster(ctx, &eks.DeleteClusterInput{Name: aws.String(clusterName)})
	if err != nil {
		return fmt.Errorf("delete EKS cluster: %w", err)
	}

	waiter := eks.NewClusterDeletedWaiter(client)
	return waiter.Wait(
		ctx,
		&eks.DescribeClusterInput{Name: aws.String(clusterName)},
		30*time.Minute,
	)
}

func (a *AWSActivities) DeleteSubnets(ctx context.Context, region, vpcID string) error {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return err
	}

	subnets, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
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

func (a *AWSActivities) DeleteRouteTables(ctx context.Context, region, vpcID string) error {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return err
	}

	rts, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
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
	region, vpcID string,
) error {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return err
	}

	igws, err := client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("attachment.vpc-id"), Values: []string{vpcID}},
		},
	})
	if err != nil || len(igws.InternetGateways) == 0 {
		return err
	}

	igwID := igws.InternetGateways[0].InternetGatewayId
	if _, err = client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
		InternetGatewayId: igwID,
		VpcId:             aws.String(vpcID),
	}); err != nil {
		return fmt.Errorf("detach IGW: %w", err)
	}

	_, err = client.DeleteInternetGateway(
		ctx,
		&ec2.DeleteInternetGatewayInput{InternetGatewayId: igwID},
	)
	return err
}

func (a *AWSActivities) DeleteVPC(ctx context.Context, region, vpcID string) error {
	client, err := newEC2Client(ctx, a, region)
	if err != nil {
		return err
	}

	_, err = client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(vpcID)})
	return err
}
