package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// Provider defines the interface for fetching node pricing.
type Provider interface {
	GetNodePrice(ctx context.Context, region, instanceType string) (float64, error)
}

// AWSClient implements Provider using AWS Pricing API.
type AWSClient struct {
	client *pricing.Client
	cache  sync.Map // map[string]float64 key=region|instanceType
}

// NewAWSClient initializes the AWS Pricing client.
// Note: AWS Pricing API is only available in us-east-1 and ap-south-1.
// We must use us-east-1 endpoint to query for all regions.
func NewAWSClient(ctx context.Context) (*AWSClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %v", err)
	}

	return &AWSClient{
		client: pricing.NewFromConfig(cfg),
	}, nil
}

func (c *AWSClient) GetNodePrice(ctx context.Context, region, instanceType string) (float64, error) {
	key := fmt.Sprintf("%s|%s", region, instanceType)
	if val, ok := c.cache.Load(key); ok {
		return val.(float64), nil
	}

	// Fetch from AWS
	price, err := c.fetchPrice(ctx, region, instanceType)
	if err != nil {
		return 0, err
	}

	c.cache.Store(key, price)
	return price, nil
}

// fetchPrice queries AWS Pricing API.
// Note: "Region" in Pricing API is "Location" attribute (e.g. "US East (N. Virginia)").
// We need to map region codes (us-east-1) to Location descriptions?
// Actually, GetProducts allows filtering by "regionCode" if we use serviceCode="AmazonEC2".
func (c *AWSClient) fetchPrice(ctx context.Context, regionCode, instanceType string) (float64, error) {
	input := &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		Filters: []types.Filter{
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("instanceType"),
				Value: aws.String(instanceType),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("regionCode"),
				Value: aws.String(regionCode),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("operatingSystem"),
				Value: aws.String("Linux"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("preInstalledSw"),
				Value: aws.String("NA"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("tenancy"),
				Value: aws.String("Shared"),
			},
			{
				Type:  types.FilterTypeTermMatch,
				Field: aws.String("capacitystatus"),
				Value: aws.String("Used"),
			},
		},
		MaxResults: aws.Int32(1),
	}

	resp, err := c.client.GetProducts(ctx, input)
	if err != nil {
		return 0, fmt.Errorf("aws pricing api error: %w", err)
	}

	if len(resp.PriceList) == 0 {
		return 0, fmt.Errorf("no price found for %s in %s", instanceType, regionCode)
	}

	// The PriceList is a list of JSON strings. We need to parse it.
	// Structure is complex: Product -> Terms -> OnDemand -> PriceDimensions -> PricePerUnit.
	priceStr := resp.PriceList[0]

	return parseAWSPriceJSON(priceStr)
}

func parseAWSPriceJSON(jsonStr string) (float64, error) {
	// Simplified parsing logic
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return 0, err
	}

	terms, ok := data["terms"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("missing terms")
	}
	onDemand, ok := terms["OnDemand"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("missing OnDemand terms")
	}

	// Iterate over the arbitrary keys (SKUs)
	for _, term := range onDemand {
		termMap, ok := term.(map[string]interface{})
		if !ok {
			continue
		}
		priceDimensions, ok := termMap["priceDimensions"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, pd := range priceDimensions {
			pdMap, ok := pd.(map[string]interface{})
			if !ok {
				continue
			}
			pricePerUnit, ok := pdMap["pricePerUnit"].(map[string]interface{})
			if !ok {
				continue
			}
			usd, ok := pricePerUnit["USD"].(string)
			if !ok {
				continue
			}
			return strconv.ParseFloat(usd, 64)
		}
	}

	return 0, fmt.Errorf("could not extract price from document")
}
