# TODO

- policy annotation
- config transformation tests
- envoy config validation
- watch only specific namespace(s)
- run against k8s ingress conformance tests
- support http01 challenge
- recover after redis wipe: currently may not be detecting that?
- potential leak of ingresses if removed while controller is unavailable:
  per controller-runtime model we do not have full list of ingresses in the system
- certificate matching: if a matching cert already exists in the databroker config, then it might be chosen
  even if tls spec says otherwise

# Done

- monitor referenced secret & service for changes
- map annotations to route props
- support TLS certs
- record ingress state change events
