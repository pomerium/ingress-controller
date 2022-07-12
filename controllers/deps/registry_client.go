package deps

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/pomerium/ingress-controller/model"
)

type trackingClient struct {
	client.Client
	model.Registry
	model.Key
}

// NewClient creates a client that watches Get requests
// and marks these objects as dependencies in the registry, including those that were not currently found
func NewClient(c client.Client, r model.Registry, k model.Key) client.Client {
	return &trackingClient{c, r, k}
}

// Get retrieves an obj for the given object key from the Kubernetes Cluster.
func (c *trackingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	dep, err := c.makeKey(key, obj)
	if err != nil {
		return fmt.Errorf("dependency key: %w", err)
	}

	c.Registry.Add(c.Key, *dep)

	err = c.Client.Get(ctx, key, obj)
	log.FromContext(ctx).V(1).Info("Get", "key", *dep, "err", err)
	return err
}

func (c *trackingClient) makeKey(name client.ObjectKey, obj client.Object) (*model.Key, error) {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return nil, fmt.Errorf("GVK was not registered for %s/%s", name, obj.GetObjectKind())
	}
	kind := gvk.Kind
	if kind == "" {
		return nil, fmt.Errorf("no Kind available for object %s", name)
	}
	return &model.Key{Kind: kind, NamespacedName: name}, nil
}
