package config

import (
	"reflect"
	"strings"

	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Redacted returns a deep copy of cfg with every string field whose JSON
// tag name matches the secret-key denylist replaced by "***". Fields tagged
// json:"-" (Password, APIToken, Username) are skipped — they never marshal
// into operator-facing output.
func Redacted(cfg *Config) Config {
	out := *cfg
	redactValue(reflect.ValueOf(&out))
	return out
}

// redactValue walks v (must be addressable) masking secret-keyed string fields.
func redactValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		redactValue(v.Elem())
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			jsonTag := f.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}
			name := strings.SplitN(jsonTag, ",", 2)[0]
			fv := v.Field(i)
			if !fv.CanSet() {
				continue
			}
			switch fv.Kind() {
			case reflect.String:
				if logutil.KeyIsSecret(name) && fv.String() != "" {
					fv.SetString("***")
				}
			case reflect.Pointer:
				if fv.IsNil() {
					continue
				}
				clone := reflect.New(fv.Type().Elem())
				clone.Elem().Set(fv.Elem())
				fv.Set(clone)
				redactValue(fv)
			case reflect.Struct:
				redactValue(fv)
			}
		}
	}
}
