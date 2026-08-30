package config

import (
	"reflect"
	"strings"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Redacted returns a deep copy of cfg with secret-keyed strings masked as
// "***"; json:"-" credential fields are already excluded and untouched here.
func Redacted(cfg *Config) Config {
	out := *cfg
	redactValue(reflect.ValueOf(&out))
	return out
}

// redactValue walks v masking secret-keyed strings, cloning any pointer,
// map, or slice it descends into so mutations never alias the source.
func redactValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		if v.CanSet() {
			clone := reflect.New(v.Type().Elem())
			clone.Elem().Set(v.Elem())
			v.Set(clone)
		}
		redactValue(v.Elem())
	case reflect.Struct:
		redactStructFields(v)
	case reflect.Map:
		if !v.IsNil() && v.CanSet() {
			v.Set(redactedMapCopy(v))
		}
	case reflect.Slice:
		if !v.IsNil() && v.CanSet() {
			clone := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			reflect.Copy(clone, v)
			for i := range clone.Len() {
				redactValue(clone.Index(i))
			}
			v.Set(clone)
		}
	}
}

func redactStructFields(v reflect.Value) {
	t := v.Type()
	for i := range t.NumField() {
		jsonTag := t.Field(i).Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if fv.Kind() == reflect.String {
			name := strings.SplitN(jsonTag, ",", 2)[0]
			if logutil.KeyIsSecret(name) && fv.String() != "" {
				fv.SetString("***")
			}
			continue
		}
		redactValue(fv)
	}
}

// redactedMapCopy returns a masked copy of m: string values under a
// secret-matching key become "***", others recurse.
func redactedMapCopy(m reflect.Value) reflect.Value {
	out := reflect.MakeMapWithSize(m.Type(), m.Len())
	for iter := m.MapRange(); iter.Next(); {
		k := iter.Key()
		elem := reflect.New(m.Type().Elem()).Elem()
		elem.Set(iter.Value())
		if elem.Kind() == reflect.String {
			if k.Kind() == reflect.String && logutil.KeyIsSecret(k.String()) && elem.String() != "" {
				elem.SetString("***")
			}
		} else {
			redactValue(elem)
		}
		out.SetMapIndex(k, elem)
	}
	return out
}
