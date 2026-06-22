package do

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/metrics"
)

// IsForbidden reports whether err is a DigitalOcean 403 (typically a token
// missing the required scope, e.g. monitoring read).
func IsForbidden(err error) bool {
	var er *godo.ErrorResponse
	if errors.As(err, &er) && er.Response != nil {
		return er.Response.StatusCode == http.StatusForbidden
	}
	return false
}

// API is the data-layer surface the rest of Sailor depends on. It is an
// interface so the UI and scheduler can be tested against a fake.
type API interface {
	// ListDropletsPaged invokes cb once per fetched page so the UI can render
	// the first page before the rest of the account is listed.
	ListDropletsPaged(ctx context.Context, cb func([]Droplet)) error
	ListDroplets(ctx context.Context) ([]Droplet, error)
	CPUSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error)
	MemAvailableSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error)
	FSFreeSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error)
	// Rate reports the most recent rate-limit headers seen on any response.
	Rate() godo.Rate
}

// Client is the godo-backed implementation of API.
type Client struct {
	g *godo.Client

	mu   sync.Mutex
	rate godo.Rate
}

// NewClient builds a Client from a DigitalOcean API token.
func NewClient(token string) *Client {
	return &Client{g: godo.NewFromToken(token)}
}

func (c *Client) observe(resp *godo.Response) {
	if resp == nil {
		return
	}
	c.mu.Lock()
	c.rate = resp.Rate
	c.mu.Unlock()
}

// Rate returns the last-seen rate-limit state.
func (c *Client) Rate() godo.Rate {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rate
}

// ListDropletsPaged fetches Droplets page by page, invoking cb for each page as
// soon as it arrives.
func (c *Client) ListDropletsPaged(ctx context.Context, cb func([]Droplet)) error {
	opt := &godo.ListOptions{PerPage: 200}
	for {
		page, resp, err := c.g.Droplets.List(ctx, opt)
		c.observe(resp)
		if err != nil {
			return err
		}
		out := make([]Droplet, 0, len(page))
		for i := range page {
			out = append(out, toDroplet(&page[i]))
		}
		cb(out)
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			return nil
		}
		next, err := resp.Links.CurrentPage()
		if err != nil {
			return nil
		}
		opt.Page = next + 1
	}
}

// ListDroplets returns every Droplet in the account (paginating internally).
func (c *Client) ListDroplets(ctx context.Context) ([]Droplet, error) {
	var out []Droplet
	err := c.ListDropletsPaged(ctx, func(page []Droplet) { out = append(out, page...) })
	if err != nil {
		return nil, err
	}
	return out, nil
}

func toDroplet(d *godo.Droplet) Droplet {
	ip, _ := d.PublicIPv4()
	return Droplet{
		ID:       d.ID,
		Name:     d.Name,
		Status:   d.Status,
		MemoryMB: d.Memory,
		DiskGB:   d.Disk,
		Vcpus:    d.Vcpus,
		SizeSlug: d.SizeSlug,
		PublicIP: ip,
	}
}

func (c *Client) metricReq(id string, start, end time.Time) *godo.DropletMetricsRequest {
	return &godo.DropletMetricsRequest{HostID: id, Start: start, End: end}
}

func (c *Client) series(
	ctx context.Context,
	id string, start, end time.Time,
	fn func(context.Context, *godo.DropletMetricsRequest) (*godo.MetricsResponse, *godo.Response, error),
) ([]metrics.SampleStream, error) {
	res, resp, err := fn(ctx, c.metricReq(id, start, end))
	c.observe(resp)
	if err != nil {
		return nil, err
	}
	return res.Data.Result, nil
}

func (c *Client) CPUSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error) {
	return c.series(ctx, id, start, end, c.g.Monitoring.GetDropletCPU)
}

func (c *Client) MemAvailableSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error) {
	return c.series(ctx, id, start, end, c.g.Monitoring.GetDropletAvailableMemory)
}

func (c *Client) FSFreeSeries(ctx context.Context, id string, start, end time.Time) ([]metrics.SampleStream, error) {
	return c.series(ctx, id, start, end, c.g.Monitoring.GetDropletFilesystemFree)
}

// IDString renders a Droplet ID as the string the metrics API expects.
func IDString(id int) string { return strconv.Itoa(id) }
