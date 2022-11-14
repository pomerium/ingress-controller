---
title: Kubernetes Deployment Reference
sidebar_label: Reference
description: Reference for Pomerium settings in Kubernetes deployments.
---

Pomerium-specific parameters should be configured via the `ingress.pomerium.io/Pomerium` CRD.
The default Pomerium deployment is listening to the CRD `global`, that may be customized via command line parameters.
Pomerium posts update to the CRD <a href="#status">`/status`</a>, and may be observed using `kubectl describe pomerium`.

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
                <code>authenticate</code>&#160;&#160;
                
                    <strong>object</strong>&#160;
                    (<a href="#authenticate">authenticate</a>)
                
                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Authenticate sets authenticate service parameters
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>certificates</code>&#160;&#160;
                
                    <strong>[]string</strong>&#160;
                
                </p>
                <p>
                    
                    Certificates is a list of secrets of type TLS to use
                </p>
                
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
                    <strong>Required.</strong>&#160;
                    IdentityProvider see https://www.pomerium.com/docs/identity-providers/
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
                    
                    JWTClaimHeaders convert claims from the assertion token into HTTP headers. We recommend you only use it for compatibility with legacy applications, and use JWT assertion header directly for new applications, read more at https://www.pomerium.com/docs/topics/getting-users-identity
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>secrets</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secrets references a Secret with Pomerium bootstrap parameters. In a default Pomerium installation manifest, they would be generated via a one-time job and stored in a <code>pomerium/bootstrap</code> Secret. You may re-run the job to rotate the secrets, or update the Secret values manually. 
 <p> <ul> <li><a href="https://pomerium.com/docs/reference/shared-secret"><code>shared_secret</code></a> - secures inter-Pomerium service communications. </li> <li><a href="https://pomerium.com/docs/reference/cookie-secret"><code>cookie_secret</code></a> - encrypts Pomerium session browser cookie. See also other <a href="#cookie">Cookie</a> parameters. </li> <li><a href="https://pomerium.com/docs/reference/signing-key"><code>signing_key</code></a> signs Pomerium JWT assertion header. See <a href="https://www.pomerium.com/docs/topics/getting-users-identity">Getting the user's identity</a> guide. </li> </ul> </p>
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
    
    </tbody>
</table>



### authenticate

Authenticate sets authenticate service parameters

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
                    
                    CallbackPath see https://www.pomerium.com/reference/#authenticate-callback-path
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
                    AuthenticateURL should be publicly accessible URL the non-authenticated persons would be referred to see https://www.pomerium.com/reference/#authenticate-service-url
                </p>
                
                    Format: an URI as parsed by Golang net/url.ParseRequestURI.
                
            </td>
        </tr>
    
    </tbody>
</table>



### cookie

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
                    
                    Domain see https://docs.pomerium.com/docs/reference/cookie-domain
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>expire</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    Expire see https://docs.pomerium.com/docs/reference/cookie-expire
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>httpOnly</code>&#160;&#160;
                
                    <strong>boolean</strong>&#160;
                
                </p>
                <p>
                    
                    HTTPOnly see https://docs.pomerium.com/docs/reference/cookie-http-only
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
                    
                    Name see https://docs.pomerium.com/docs/reference/cookie-name
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
                    
                    Secure see https://docs.pomerium.com/docs/reference/cookie-secure
                </p>
                
            </td>
        </tr>
    
    </tbody>
</table>



### identityProvider

IdentityProvider see https://www.pomerium.com/docs/identity-providers/

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
                    
                    RefreshDirectory is no longer supported, please see https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync
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
                    
                    RequestParams to be added as part of a signin request using OAuth2 code flow.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>requestParamsSecret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    RequestParamsSecret is a reference to a secret for additional parameters you'd prefer not to provide in plaintext.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>scopes</code>&#160;&#160;
                
                    <strong>[]string</strong>&#160;
                
                </p>
                <p>
                    
                    Scopes correspond to access <a href="https://www.pomerium.com/reference/#identity-provider-scopes">privilege scopes</a>.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>secret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secret containing IdP provider specific parameters and must contain at least <code>client_id</code> and <code>client_secret</code> values.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>serviceAccountFromSecret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    ServiceAccountFromSecret is no longer supported, see <a href="https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync">Upgrade Guide</a>
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



### postgres

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
                
                </p>
                <p>
                    
                    CASecret should refer to a k8s secret with key `ca.crt` containing CA certificate that, if specified, would be used to populate `sslrootcert` parameter of the connection string
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>secret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secret specifies a name of a Secret that must contain <code>connection</code> key. See <a href="https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING">DSN Format and Parameters</a>. Keywords related to TLS are not allowed, as they must be populated via <code>tlsCecret</code> and <code>caSecret</code> fields.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>tlsSecret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    TLSSecret should refer to a k8s secret of type `kubernetes.io/tls` and allows to specify an optional client certificate and key, by constructing `sslcert` and `sslkey` connection string parameter values see https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-PARAMKEYWORDS
                </p>
                
            </td>
        </tr>
    
    </tbody>
</table>



### redis

Redis defines REDIS connection parameters

<table>
    <thead>
    </thead>
    <tbody>
    
        <tr>
            <td>
                <p>
                <code>caSecret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    CASecret should refer to a k8s secret with key <code>ca.crt</code> that must be a PEM-encoded certificate authority to use when connecting to the databroker storage engine.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>secret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    <strong>Required.</strong>&#160;
                    Secret specifies a name of a Secret that must contain <code>connection</code> key.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>tlsSecret</code>&#160;&#160;
                
                    <strong>string</strong>&#160;
                
                </p>
                <p>
                    
                    TLSSecret should refer to a k8s secret of type <code>kubernetes.io/tls</code> that would be used to perform TLS connection to REDIS.
                </p>
                
            </td>
        </tr>
    
        <tr>
            <td>
                <p>
                <code>tlsSkipVerify</code>&#160;&#160;
                
                    <strong>boolean</strong>&#160;
                
                </p>
                <p>
                    
                    TLSSkipVerify disables TLS certificate chain validation.
                </p>
                
            </td>
        </tr>
    
    </tbody>
</table>



### refreshDirectory

RefreshDirectory is no longer supported, please see https://docs.pomerium.com/docs/overview/upgrading#idp-directory-sync

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
                
                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration
                
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
                
                    Format: a duration string like "22s" as parsed by Golang time.ParseDuration
                
            </td>
        </tr>
    
    </tbody>
</table>



### storage

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
    
        <tr>
            <td>
                <p>
                <code>redis</code>&#160;&#160;
                
                    <strong>object</strong>&#160;
                    (<a href="#redis">redis</a>)
                
                </p>
                <p>
                    
                    Redis defines REDIS connection parameters
                </p>
                
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



### ingress

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



### settingsStatus

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



