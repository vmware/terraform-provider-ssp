package provider

import (
	"fmt"
	"os"
	"sync"

	"github.com/vmware/terraform-provider-ssp/internal/client/api_client"
	"github.com/vmware/terraform-provider-ssp/internal/client/depot_client"
	"github.com/vmware/terraform-provider-ssp/internal/client/iam_client"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

// ClientData is the shared provider configuration passed to resources and data sources.
type ClientData struct {
	// SSPI Appliance Clients (Day-0 / Day-1)
	API   *api_client.ClientWithResponses
	Depot *depot_client.ClientWithResponses
	IAM   *iam_client.ClientWithResponses

	// SSP Runtime Client parameters (Day-2)
	RuntimeHost     string
	RuntimeUsername string
	RuntimePassword string
	RuntimeInsecure bool

	runtimeClient *client.Client
	mu            sync.Mutex
}

func (c *ClientData) GetAPI() *api_client.ClientWithResponses     { return c.API }
func (c *ClientData) GetDepot() *depot_client.ClientWithResponses { return c.Depot }
func (c *ClientData) GetIAM() *iam_client.ClientWithResponses     { return c.IAM }

// GetSSPClient returns the authenticated HTTP client for SSP runtime APIs (lazy initialized).
func (c *ClientData) GetSSPClient() (*client.Client, error) {
	if c == nil {
		return nil, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.runtimeClient != nil {
		return c.runtimeClient, nil
	}

	host := c.RuntimeHost
	if host == "" {
		host = os.Getenv("SSP_HOST")
	}
	username := c.RuntimeUsername
	if username == "" {
		username = os.Getenv("SSP_USERNAME")
	}
	password := c.RuntimePassword
	if password == "" {
		password = os.Getenv("SSP_PASSWORD")
	}

	if host == "" {
		return nil, nil
	}

	c.runtimeClient = client.NewClient(host, username, password, c.RuntimeInsecure)
	return c.runtimeClient, nil
}

// GetClientFromProviderData helper extracts or lazily retrieves *client.Client from req.ProviderData.
func GetClientFromProviderData(providerData any) (*client.Client, error) {
	if providerData == nil {
		return nil, nil
	}
	if c, ok := providerData.(*client.Client); ok {
		return c, nil
	}
	if cd, ok := providerData.(*ClientData); ok {
		c, err := cd.GetSSPClient()
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, fmt.Errorf(
				"the SSP runtime cluster connection is not configured: set the provider's " +
					"\"host\", \"username\", and \"password\" attributes (or the SSP_HOST, SSP_USERNAME, " +
					"and SSP_PASSWORD environment variables) to use this resource or data source",
			)
		}
		return c, nil
	}
	return nil, fmt.Errorf("expected *client.Client or *ClientData, got %T", providerData)
}
