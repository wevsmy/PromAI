package prometheus

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	api v1.API
}

func NewClient(url string) (*Client, error) {
	client, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		api: v1.NewAPI(client),
	}, nil
}

func (c *Client) Query(query string) (model.Value, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}

	return result, nil
}
