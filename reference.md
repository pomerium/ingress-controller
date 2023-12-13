---
title: Kubernetes Deployment Reference
sidebar_label: Reference
description: Reference for Pomerium settings in Kubernetes deployments.
---

Pomerium-specific parameters should be configured via the `ingress.pomerium.io/Pomerium` CRD.
The default Pomerium deployment is listening to the CRD `global`, that may be customized via command line parameters.

Pomerium posts updates to the CRD <a href="#status">`/status`</a>:
```shell
kubectl describe pomerium
```

Kubernetes-specific deployment parameters should be added via `kustomize` to the manifests.

## Spec

PomeriumSpec defines Pomerium-specific configuration parameters.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>accessLogFields</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    AccessLogFields sets the <a href="https://www.pomerium.com/docs/reference/access-log-fields">access fields</a> to log.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>authenticate</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#authenticate">authenticate</a>)

                </p>
                <p>

                    Authenticate sets authenticate service parameters. If not specified, a Pomerium-hosted authenticate service would be used.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>authorizeLogFields</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    AuthorizeLogFields sets the <a href="https://www.pomerium.com/docs/reference/authorize-log-fields">authorize fields</a> to log.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>caSecrets</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    CASecret should refer to k8s secrets with key <code>ca.crt</code> containing a CA certificate.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>certificates</code>&#160;&#160;

                    <strong>[]string</strong>&#160;
                    (namespace/name)

                </p>
                <p>

                    Certificates is a list of secrets of type TLS to use
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>cookie</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#cookie">cookie</a>)

                </p>
                <p>

                    Cookie defines Pomerium session cookie options.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>identityProvider</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#identityprovider">identityProvider</a>)

                </p>
                <p>

                    IdentityProvider configure single-sign-on authentication and user identity details by integrating with your <a href="https://www.pomerium.com/docs/identity-providers/">Identity Provider</a>
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>jwtClaimHeaders</code>&#160;&#160;

                    <strong>map[string]string</strong>

                </p>
                <p>

                    JWTClaimHeaders convert claims from the assertion token into HTTP headers and adds them into JWT assertion header. Please make sure to read <a href="https://www.pomerium.com/docs/topics/getting-users-identity"> Getting User Identity</a> guide.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>passIdentityHeaders</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>

                    PassIdentityHeaders sets the <a href="https://www.pomerium.com/docs/reference/routes/pass-identity-headers">pass identity headers</a> option.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>programmaticRedirectDomains</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    ProgrammaticRedirectDomains specifies a list of domains that can be used for <a href="https://www.pomerium.com/docs/capabilities/programmatic-access">programmatic redirects</a>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>secrets</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secrets references a Secret with Pomerium bootstrap parameters.
 <p> <ul> <li><a href="https://pomerium.com/docs/reference/shared-secret"><code>shared_secret</code></a> - secures inter-Pomerium service communications. </li> <li><a href="https://pomerium.com/docs/reference/cookie-secret"><code>cookie_secret</code></a> - encrypts Pomerium session browser cookie. See also other <a href="#cookie">Cookie</a> parameters. </li> <li><a href="https://pomerium.com/docs/reference/signing-key"><code>signing_key</code></a> signs Pomerium JWT assertion header. See <a href="https://www.pomerium.com/docs/topics/getting-users-identity">Getting the user's identity</a> guide. </li> </ul> </p> <p> In a default Pomerium installation manifest, they would be generated via a <a href="https://github.com/pomerium/ingress-controller/blob/main/config/gen_secrets/job.yaml">one-time job</a> and stored in a <code>pomerium/bootstrap</code> Secret. You may re-run the job to rotate the secrets, or update the Secret values manually. </p>
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>setResponseHeaders</code>&#160;&#160;

                    <strong>map[string]string</strong>

                </p>
                <p>

                    SetResponseHeaders specifies a mapping of HTTP Header to be added globally to all managed routes and pomerium's authenticate service. See <a href="https://www.pomerium.com/docs/reference/set-response-headers">Set Response Headers</a>
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>storage</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#storage">storage</a>)

                </p>
                <p>

                    Storage defines persistent storage for sessions and other data. See <a href="https://www.pomerium.com/docs/topics/data-storage">Storage</a> for details. If no storage is specified, Pomerium would use a transient in-memory storage (not recommended for production).
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>timeouts</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#timeouts">timeouts</a>)

                </p>
                <p>

                    Timeout specifies the <a href="https://www.pomerium.com/docs/reference/global-timeouts">global timeouts</a> for all routes.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>useProxyProtocol</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>

                    UseProxyProtocol enables <a href="https://www.pomerium.com/docs/reference/use-proxy-protocol">Proxy Protocol</a> support.
                </p>

            </td>
        </tr>

    </tbody>
</table>



### `authenticate`

Authenticate sets authenticate service parameters. If not specified, a Pomerium-hosted authenticate service would be used.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>callbackPath</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    CallbackPath sets the path at which the authenticate service receives callback responses from your identity provider. The value must exactly match one of the authorized redirect URIs for the OAuth 2.0 client.
 <p>This value is referred to as the redirect_url in the OpenIDConnect and OAuth2 specs.</p> <p>Defaults to <code>/oauth2/callback</code></p>
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>url</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (uri)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    AuthenticateURL is a dedicated domain URL the non-authenticated persons would be referred to.
 <p><ul> <li>You do not need to create a dedicated <code>Ingress</code> for this virtual route, as it is handled by Pomerium internally. </li> <li>You do need create a secret with corresponding TLS certificate for this route and reference it via <a href="#prop-certificates"><code>certificates</code></a>. If you use <code>cert-manager</code> with <code>HTTP01</code> challenge, you may use <code>pomerium</code> <code>ingressClass</code> to solve it.</li> </ul></p>
                </p>

                    Format: an URI as parsed by Golang net/url.ParseRequestURI.

            </td>
        </tr>

    </tbody>
</table>



### `cookie`

Cookie defines Pomerium session cookie options.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>domain</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    Domain defaults to the same host that set the cookie. If you specify the domain explicitly, then subdomains would also be included.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>expire</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>

                    Expire sets cookie and Pomerium session expiration time. Once session expires, users would have to re-login. If you change this parameter, existing sessions are not affected. <p>See <a href="https://www.pomerium.com/docs/enterprise/about#session-management">Session Management</a> (Enterprise) for a more fine-grained session controls.</p> <p>Defaults to 14 hours.</p>
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>httpOnly</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>

                    HTTPOnly if set to <code>false</code>, the cookie would be accessible from within the JavaScript. Defaults to <code>true</code>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>name</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    Name sets the Pomerium session cookie name. Defaults to <code>_pomerium</code>
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>sameSite</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    SameSite sets the SameSite option for cookies. Defaults to <code></code>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>secure</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>

                    Secure if set to false, would make a cookie accessible over insecure protocols (HTTP). Defaults to <code>true</code>.
                </p>

            </td>
        </tr>

    </tbody>
</table>



### `identityProvider`

IdentityProvider configure single-sign-on authentication and user identity details by integrating with your <a href="https://www.pomerium.com/docs/identity-providers/">Identity Provider</a>

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>provider</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Provider is the short-hand name of a built-in OpenID Connect (oidc) identity provider to be used for authentication. To use a generic provider, set to <code>oidc</code>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>refreshDirectory</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#refreshdirectory">refreshDirectory</a>)

                </p>
                <p>

                    RefreshDirectory is no longer supported, please see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>requestParams</code>&#160;&#160;

                    <strong>map[string]string</strong>

                </p>
                <p>

                    RequestParams to be added as part of a sign-in request using OAuth2 code flow.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>requestParamsSecret</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>

                    RequestParamsSecret is a reference to a secret for additional parameters you'd prefer not to provide in plaintext.
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>scopes</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    Scopes Identity provider scopes correspond to access privilege scopes as defined in Section 3.3 of OAuth 2.0 RFC6749.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>secret</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secret containing IdP provider specific parameters. and must contain at least <code>client_id</code> and <code>client_secret</code> values.
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>serviceAccountFromSecret</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    ServiceAccountFromSecret is no longer supported, see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>url</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (uri)

                </p>
                <p>

                    URL is the base path to an identity provider's OpenID connect discovery document. See <a href="https://pomerium.com/docs/identity-providers">Identity Providers</a> guides for details.
                </p>

                    Format: an URI as parsed by Golang net/url.ParseRequestURI.

            </td>
        </tr>

    </tbody>
</table>



### `postgres`

Postgres specifies PostgreSQL database connection parameters

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>caSecret</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>

                    CASecret should refer to a k8s secret with key <code>ca.crt</code> containing CA certificate that, if specified, would be used to populate <code>sslrootcert</code> parameter of the connection string.
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>secret</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secret specifies a name of a Secret that must contain <code>connection</code> key. See <a href="https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING">DSN Format and Parameters</a>. Do not set <code>sslrootcert</code>, <code>sslcert</code> and <code>sslkey</code> via connection string, use <code>tlsSecret</code> and <code>caSecret</code> CRD options instead.
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>tlsSecret</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (namespace/name)

                </p>
                <p>

                    TLSSecret should refer to a k8s secret of type <code>kubernetes.io/tls</code> and allows to specify an optional client certificate and key, by constructing <code>sslcert</code> and <code>sslkey</code> connection string <a href="https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS"> parameter values</a>.
                </p>

                    Format: reference to Kubernetes resource with namespace prefix: <code>namespace/name</code> format.

            </td>
        </tr>

    </tbody>
</table>



### `refreshDirectory`

RefreshDirectory is no longer supported, please see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>interval</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    interval is the time that pomerium will sync your IDP directory.
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>timeout</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    timeout is the maximum time allowed each run.
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

    </tbody>
</table>



### `storage`

Storage defines persistent storage for sessions and other data. See <a href="https://www.pomerium.com/docs/topics/data-storage">Storage</a> for details. If no storage is specified, Pomerium would use a transient in-memory storage (not recommended for production).

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>postgres</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#postgres">postgres</a>)

                </p>
                <p>

                    Postgres specifies PostgreSQL database connection parameters
                </p>

            </td>
        </tr>

    </tbody>
</table>



### `timeouts`

Timeout specifies the <a href="https://www.pomerium.com/docs/reference/global-timeouts">global timeouts</a> for all routes.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>idle</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>

                    Idle specifies the time at which a downstream or upstream connection will be terminated if there are no active streams.
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>read</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>

                    Read specifies the amount of time for the entire request stream to be received from the client.
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>write</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (duration)

                </p>
                <p>

                    Write specifies max stream duration is the maximum time that a stream’s lifetime will span. An HTTP request/response exchange fully consumes a single stream. Therefore, this value must be greater than read_timeout as it covers both request and response time.
                </p>

                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration.

            </td>
        </tr>

    </tbody>
</table>



## Status

PomeriumStatus represents configuration and Ingress status.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>ingress</code>&#160;&#160;

                    <strong>map[string]</strong>
                    <a href="#ingress">ingress</a>

                </p>
                <p>

                    Routes provide per-Ingress status.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>settingsStatus</code>&#160;&#160;

                    <strong>object</strong>&#160;
                    (<a href="#settingsstatus">settingsStatus</a>)

                </p>
                <p>

                    SettingsStatus represent most recent main configuration reconciliation status.
                </p>

            </td>
        </tr>

    </tbody>
</table>



### `ingress`

ResourceStatus represents the outcome of the latest attempt to reconcile relevant Kubernetes resource with Pomerium.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>error</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    Error that prevented latest observedGeneration to be synchronized with Pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>observedAt</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (date-time)

                </p>
                <p>

                    ObservedAt is when last reconciliation attempt was made.
                </p>

                    Format: a date time string like "2014-12-15T19:30:20.000Z" as defined by date-time in RFC3339.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>observedGeneration</code>&#160;&#160;

                    <strong>integer</strong>&#160;

                </p>
                <p>

                    ObservedGeneration represents the <code>.metadata.generation</code> that was last presented to Pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>reconciled</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Reconciled is whether this object generation was successfully synced with pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>warnings</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    Warnings while parsing the resource.
                </p>

            </td>
        </tr>

    </tbody>
</table>



### `settingsStatus`

SettingsStatus represent most recent main configuration reconciliation status.

<table>
    <thead>
    </thead>
    <tbody>

        <tr>
            <td>
                <p>
                <code>error</code>&#160;&#160;

                    <strong>string</strong>&#160;

                </p>
                <p>

                    Error that prevented latest observedGeneration to be synchronized with Pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>observedAt</code>&#160;&#160;

                    <strong>string</strong>&#160;
                    (date-time)

                </p>
                <p>

                    ObservedAt is when last reconciliation attempt was made.
                </p>

                    Format: a date time string like "2014-12-15T19:30:20.000Z" as defined by date-time in RFC3339.

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>observedGeneration</code>&#160;&#160;

                    <strong>integer</strong>&#160;

                </p>
                <p>

                    ObservedGeneration represents the <code>.metadata.generation</code> that was last presented to Pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>reconciled</code>&#160;&#160;

                    <strong>boolean</strong>&#160;

                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Reconciled is whether this object generation was successfully synced with pomerium.
                </p>

            </td>
        </tr>

        <tr>
            <td>
                <p>
                <code>warnings</code>&#160;&#160;

                    <strong>[]string</strong>&#160;

                </p>
                <p>

                    Warnings while parsing the resource.
                </p>

            </td>
        </tr>

    </tbody>
</table>
