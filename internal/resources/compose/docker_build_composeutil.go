package compose

import (
	"reflect"
)

// Portions of this file are adapted from github.com/compose-spec/compose-go/v2/loader/mapstructure.go
// Copyright 2020 The Compose Specification Authors
// Licensed under the Apache License, Version 2.0
// See: https://github.com/compose-spec/compose-go/blob/v2.9.0/LICENSE

// decoder is an interface for types that implement DecodeMapstructure
// This is used by decoderHook to call DecodeMapstructure method for custom types
//
// This interface is adapted from github.com/compose-spec/compose-go/v2/loader/mapstructure.go
// Copyright 2020 The Compose Specification Authors
// Licensed under the Apache License, Version 2.0
type decoder interface {
	DecodeMapstructure(any) error
}

// decoderHook is a mapstructure decode hook that calls DecodeMapstructure method
// for types that implement the decoder interface. This is required to support
// custom types like MappingWithEquals that can decode from both array and map formats.
//
// This function is adapted from github.com/compose-spec/compose-go/v2/loader/mapstructure.go
// Copyright 2020 The Compose Specification Authors
// Licensed under the Apache License, Version 2.0
//
// See: https://github.com/mitchellh/mapstructure/issues/115#issuecomment-735287466
// Adapted to support types derived from built-in types, as DecodeMapstructure would not be able
// to mutate internal value, so need to invoke DecodeMapstructure defined by pointer to type
func decoderHook(from reflect.Value, to reflect.Value) (any, error) {
	// If the destination implements the decoder interface
	u, ok := to.Interface().(decoder)
	if !ok {
		// for non-struct types we need to invoke func (*type) DecodeMapstructure()
		if to.CanAddr() {
			pto := to.Addr()
			u, ok = pto.Interface().(decoder)
		}
		if !ok {
			return from.Interface(), nil
		}
	}
	// If it is nil and a pointer, create and assign the target value first
	if to.Type().Kind() == reflect.Ptr && to.IsNil() {
		to.Set(reflect.New(to.Type().Elem()))
		u = to.Interface().(decoder)
	}
	// Call the custom DecodeMapstructure method
	if err := u.DecodeMapstructure(from.Interface()); err != nil {
		return to.Interface(), err
	}
	return to.Interface(), nil
}
