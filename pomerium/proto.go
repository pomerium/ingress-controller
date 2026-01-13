package pomerium

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"gopkg.in/yaml.v3"
)

func unmarshalAnnotations(dst proto.Message, kvs map[string]string) error {
	// first convert the map[string]string to a map[string]any via yaml
	src := make(map[string]any, len(kvs))
	for k, v := range kvs {
		var out any
		if err := yaml.Unmarshal([]byte(v), &out); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		src[k] = out
	}

	// pre-process the json to handle custom formats
	preprocessAnnotationMessage(dst.ProtoReflect().Descriptor(), src)

	// marshal as json so it can be unmarshaled via protojson
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(data, dst)
}

func preprocessAnnotationMessage(md protoreflect.MessageDescriptor, data any) any {
	name := md.FullName()
	switch name {
	case "google.protobuf.Duration":
		// convert go duration strings into protojson duration strings
		if v, ok := data.(string); ok {
			return goDurationStringToProtoJSONDurationString(v)
		}
	case "pomerium.config.Route.StringList":
		if v, ok := data.([]any); ok {
			return map[string]any{"values": v}
		}
	case "envoy.type.v3.Percent":
		// convert percentage value to percentage message {"value" : <double>}
		v, ok := data.(float64)
		if ok {
			return map[string]float64{
				"value": v,
			}
		}
		fallthrough
	default:
		// preprocess all the fields
		if v, ok := data.(map[string]any); ok {
			fds := md.Fields()
			for i := 0; i < fds.Len(); i++ {
				fd := fds.Get(i)
				name := string(fd.Name())
				vv, ok := v[name]
				if ok {
					v[name] = preprocessAnnotationField(fd, vv)
				}
			}
			return v
		}
	}
	return data
}

func preprocessAnnotationField(fd protoreflect.FieldDescriptor, data any) any {
	if fd.Enum() != nil && fd.Enum().FullName() == "pomerium.config.LoadBalancingPolicy" {
		if v, ok := data.(string); ok {
			switch strings.ToLower(v) {
			case "", "load_balancing_policy_unspecified", "unspecified":
				return "LOAD_BALANCING_POLICY_UNSPECIFIED"
			case "load_balancing_policy_round_robin", "round_robin":
				return "LOAD_BALANCING_POLICY_ROUND_ROBIN"
			case "load_balancing_policy_maglev", "maglev":
				return "LOAD_BALANCING_POLICY_MAGLEV"
			case "load_balancing_policy_random", "random":
				return "LOAD_BALANCING_POLICY_RANDOM"
			case "load_balancing_policy_ring_hash", "ring_hash":
				return "LOAD_BALANCING_POLICY_RING_HASH"
			case "load_balancing_policy_least_request", "least_request":
				return "LOAD_BALANCING_POLICY_LEAST_REQUEST"
			}
		}
	}
	if fd.Enum() != nil && fd.Enum().FullName() == "pomerium.config.BearerTokenFormat" {
		if v, ok := data.(string); ok {
			switch v {
			case "":
				return "BEARER_TOKEN_FORMAT_UNKNOWN"
			case "default":
				return "BEARER_TOKEN_FORMAT_DEFAULT"
			case "idp_access_token":
				return "BEARER_TOKEN_FORMAT_IDP_ACCESS_TOKEN"
			case "idp_identity_token":
				return "BEARER_TOKEN_FORMAT_IDP_IDENTITY_TOKEN"
			}
		}
	}
	// if this is a repeated field, handle each of the field values separately
	if fd.IsList() {
		vs, ok := data.([]any)
		if ok {
			nvs := make([]any, len(vs))
			for i, v := range vs {
				nvs[i] = preprocessAnnotationFieldValue(fd, v)
			}
			return nvs
		}
	}

	return preprocessAnnotationFieldValue(fd, data)
}

func preprocessAnnotationFieldValue(fd protoreflect.FieldDescriptor, data any) any {
	// convert map[string]any -> map[string]string
	if fd.IsMap() && fd.MapKey().Kind() == protoreflect.StringKind && fd.MapValue().Kind() == protoreflect.StringKind {
		if v, ok := data.(map[string]any); ok {
			m := make(map[string]string, len(v))
			for k, vv := range v {
				m[k] = fmt.Sprint(vv)
			}
			return m
		}
	}

	switch fd.Kind() {
	case protoreflect.MessageKind:
		return preprocessAnnotationMessage(fd.Message(), data)
	}

	return data
}

func goDurationStringToProtoJSONDurationString(in string) string {
	dur, err := time.ParseDuration(in)
	if err != nil {
		return in
	}

	bs, err := protojson.Marshal(durationpb.New(dur))
	if err != nil {
		return in
	}

	str := strings.Trim(string(bs), `"`)
	return str
}
