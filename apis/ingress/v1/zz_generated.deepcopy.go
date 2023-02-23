//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Authenticate) DeepCopyInto(out *Authenticate) {
	*out = *in
	if in.CallbackPath != nil {
		in, out := &in.CallbackPath, &out.CallbackPath
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Authenticate.
func (in *Authenticate) DeepCopy() *Authenticate {
	if in == nil {
		return nil
	}
	out := new(Authenticate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Cookie) DeepCopyInto(out *Cookie) {
	*out = *in
	if in.Name != nil {
		in, out := &in.Name, &out.Name
		*out = new(string)
		**out = **in
	}
	if in.Domain != nil {
		in, out := &in.Domain, &out.Domain
		*out = new(string)
		**out = **in
	}
	if in.Secure != nil {
		in, out := &in.Secure, &out.Secure
		*out = new(bool)
		**out = **in
	}
	if in.HTTPOnly != nil {
		in, out := &in.HTTPOnly, &out.HTTPOnly
		*out = new(bool)
		**out = **in
	}
	if in.Expire != nil {
		in, out := &in.Expire, &out.Expire
		*out = new(metav1.Duration)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Cookie.
func (in *Cookie) DeepCopy() *Cookie {
	if in == nil {
		return nil
	}
	out := new(Cookie)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IdentityProvider) DeepCopyInto(out *IdentityProvider) {
	*out = *in
	if in.URL != nil {
		in, out := &in.URL, &out.URL
		*out = new(string)
		**out = **in
	}
	if in.ServiceAccountFromSecret != nil {
		in, out := &in.ServiceAccountFromSecret, &out.ServiceAccountFromSecret
		*out = new(string)
		**out = **in
	}
	if in.RequestParams != nil {
		in, out := &in.RequestParams, &out.RequestParams
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.RequestParamsSecret != nil {
		in, out := &in.RequestParamsSecret, &out.RequestParamsSecret
		*out = new(string)
		**out = **in
	}
	if in.Scopes != nil {
		in, out := &in.Scopes, &out.Scopes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.RefreshDirectory != nil {
		in, out := &in.RefreshDirectory, &out.RefreshDirectory
		*out = new(RefreshDirectorySettings)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IdentityProvider.
func (in *IdentityProvider) DeepCopy() *IdentityProvider {
	if in == nil {
		return nil
	}
	out := new(IdentityProvider)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Pomerium) DeepCopyInto(out *Pomerium) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Pomerium.
func (in *Pomerium) DeepCopy() *Pomerium {
	if in == nil {
		return nil
	}
	out := new(Pomerium)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Pomerium) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PomeriumList) DeepCopyInto(out *PomeriumList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Pomerium, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PomeriumList.
func (in *PomeriumList) DeepCopy() *PomeriumList {
	if in == nil {
		return nil
	}
	out := new(PomeriumList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PomeriumList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PomeriumSpec) DeepCopyInto(out *PomeriumSpec) {
	*out = *in
	in.Authenticate.DeepCopyInto(&out.Authenticate)
	if in.IdentityProvider != nil {
		in, out := &in.IdentityProvider, &out.IdentityProvider
		*out = new(IdentityProvider)
		(*in).DeepCopyInto(*out)
	}
	if in.Certificates != nil {
		in, out := &in.Certificates, &out.Certificates
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.CASecrets != nil {
		in, out := &in.CASecrets, &out.CASecrets
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Storage != nil {
		in, out := &in.Storage, &out.Storage
		*out = new(Storage)
		(*in).DeepCopyInto(*out)
	}
	if in.Cookie != nil {
		in, out := &in.Cookie, &out.Cookie
		*out = new(Cookie)
		(*in).DeepCopyInto(*out)
	}
	if in.JWTClaimHeaders != nil {
		in, out := &in.JWTClaimHeaders, &out.JWTClaimHeaders
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PomeriumSpec.
func (in *PomeriumSpec) DeepCopy() *PomeriumSpec {
	if in == nil {
		return nil
	}
	out := new(PomeriumSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PomeriumStatus) DeepCopyInto(out *PomeriumStatus) {
	*out = *in
	if in.Routes != nil {
		in, out := &in.Routes, &out.Routes
		*out = make(map[string]ResourceStatus, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.SettingsStatus != nil {
		in, out := &in.SettingsStatus, &out.SettingsStatus
		*out = new(ResourceStatus)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PomeriumStatus.
func (in *PomeriumStatus) DeepCopy() *PomeriumStatus {
	if in == nil {
		return nil
	}
	out := new(PomeriumStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgresStorage) DeepCopyInto(out *PostgresStorage) {
	*out = *in
	if in.TLSSecret != nil {
		in, out := &in.TLSSecret, &out.TLSSecret
		*out = new(string)
		**out = **in
	}
	if in.CASecret != nil {
		in, out := &in.CASecret, &out.CASecret
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgresStorage.
func (in *PostgresStorage) DeepCopy() *PostgresStorage {
	if in == nil {
		return nil
	}
	out := new(PostgresStorage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStorage) DeepCopyInto(out *RedisStorage) {
	*out = *in
	if in.TLSSecret != nil {
		in, out := &in.TLSSecret, &out.TLSSecret
		*out = new(string)
		**out = **in
	}
	if in.CASecret != nil {
		in, out := &in.CASecret, &out.CASecret
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStorage.
func (in *RedisStorage) DeepCopy() *RedisStorage {
	if in == nil {
		return nil
	}
	out := new(RedisStorage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RefreshDirectorySettings) DeepCopyInto(out *RefreshDirectorySettings) {
	*out = *in
	out.Interval = in.Interval
	out.Timeout = in.Timeout
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RefreshDirectorySettings.
func (in *RefreshDirectorySettings) DeepCopy() *RefreshDirectorySettings {
	if in == nil {
		return nil
	}
	out := new(RefreshDirectorySettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceStatus) DeepCopyInto(out *ResourceStatus) {
	*out = *in
	in.ObservedAt.DeepCopyInto(&out.ObservedAt)
	if in.Error != nil {
		in, out := &in.Error, &out.Error
		*out = new(string)
		**out = **in
	}
	if in.Warnings != nil {
		in, out := &in.Warnings, &out.Warnings
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceStatus.
func (in *ResourceStatus) DeepCopy() *ResourceStatus {
	if in == nil {
		return nil
	}
	out := new(ResourceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Storage) DeepCopyInto(out *Storage) {
	*out = *in
	if in.Redis != nil {
		in, out := &in.Redis, &out.Redis
		*out = new(RedisStorage)
		(*in).DeepCopyInto(*out)
	}
	if in.Postgres != nil {
		in, out := &in.Postgres, &out.Postgres
		*out = new(PostgresStorage)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Storage.
func (in *Storage) DeepCopy() *Storage {
	if in == nil {
		return nil
	}
	out := new(Storage)
	in.DeepCopyInto(out)
	return out
}
