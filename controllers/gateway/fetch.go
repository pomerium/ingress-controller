package gateway

/*func FetchGateway(ctx context.Context, client client.Client, name types.NamespacedName) (*model.Config, error) {
	var cfg model.Config
	if err := client.Get(ctx, name, &cfg.Pomerium); err != nil {
		return nil, fmt.Errorf("get %s: %w", name, err)
	}

	if err := fetchConfigSecrets(ctx, client, &cfg); err != nil {
		return &cfg, fmt.Errorf("secrets: %w", err)
	}

	if err := fetchConfigCerts(ctx, client, &cfg); err != nil {
		return &cfg, fmt.Errorf("certs: %w", err)
	}

	return &cfg, nil
}*/
