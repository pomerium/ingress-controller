This is an example configuration how you can deploy another version of Pomerium Ingress Controller into the cluster,
which may be useful if you're testing a new version upgrade.

# Configuration

Each deployment of Pomerium should have their own global settings.
Make sure different deployments of Pomerium never share the same database if you use persistent storage.

```yaml
apiVersion: ingress.pomerium.io/v1
kind: Pomerium
metadata:
  name: global-2
spec:
  runtimeFlags:
    mcp: true
  secrets: pomerium-2/bootstrap
```
