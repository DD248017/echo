// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	"encoding"
	"encoding/xml"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// Binder is the interface that wraps the Bind method.
type Binder interface {
	Bind(i interface{}, c Context) error
}

// DefaultBinder is the default implementation of the Binder interface.
type DefaultBinder struct{}

// BindUnmarshaler is the interface used to wrap the UnmarshalParam method.
// Types that don't implement this, but do implement encoding.TextUnmarshaler
// will use that interface instead.
type BindUnmarshaler interface {
	// UnmarshalParam decodes and assigns a value from an form or query param.
	UnmarshalParam(param string) error
}

// bindMultipleUnmarshaler is used by binder to unmarshal multiple values from request at once to
// type implementing this interface. For example request could have multiple query fields `?a=1&a=2&b=test` in that case
// for `a` following slice `["1", "2"] will be passed to unmarshaller.
type bindMultipleUnmarshaler interface {
	UnmarshalParams(params []string) error
}

// BindPathParams binds path params to bindable object
func (b *DefaultBinder) BindPathParams(c Context, i interface{}) error {
	names := c.ParamNames()
	values := c.ParamValues()
	params := map[string][]string{}
	for i, name := range names {
		params[name] = []string{values[i]}
	}
	if err := b.bindData(i, params, "param", nil); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
	}
	return nil
}

// BindQueryParams binds query params to bindable object
func (b *DefaultBinder) BindQueryParams(c Context, i interface{}) error {
	if err := b.bindData(i, c.QueryParams(), "query", nil); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
	}
	return nil
}

// BindBody binds request body contents to bindable object
// NB: then binding forms take note that this implementation uses standard library form parsing
// which parses form data from BOTH URL and BODY if content type is not MIMEMultipartForm
// See non-MIMEMultipartForm: https://golang.org/pkg/net/http/#Request.ParseForm
// See MIMEMultipartForm: https://golang.org/pkg/net/http/#Request.ParseMultipartForm
func (b *DefaultBinder) BindBody(c Context, i interface{}) (err error) {
	req := c.Request()
	if req.ContentLength == 0 {
		return
	}

	// mediatype is found like `mime.ParseMediaType()` does it
	base, _, _ := strings.Cut(req.Header.Get(HeaderContentType), ";")
	mediatype := strings.TrimSpace(base)

	switch mediatype {
	case MIMEApplicationJSON:
		if err = c.Echo().JSONSerializer.Deserialize(c, i); err != nil {
			switch err.(type) {
			case *HTTPError:
				return err
			default:
				return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
			}
		}
	case MIMEApplicationXML, MIMETextXML:
		if err = xml.NewDecoder(req.Body).Decode(i); err != nil {
			if ute, ok := err.(*xml.UnsupportedTypeError); ok {
				return NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unsupported type error: type=%v, error=%v", ute.Type, ute.Error())).SetInternal(err)
			} else if se, ok := err.(*xml.SyntaxError); ok {
				return NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Syntax error: line=%v, error=%v", se.Line, se.Error())).SetInternal(err)
			}
			return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
		}
	case MIMEApplicationForm:
		params, err := c.FormParams()
		if err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
		}
		if err = b.bindData(i, params, "form", nil); err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
		}
	case MIMEMultipartForm:
		params, err := c.MultipartForm()
		if err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
		}
		if err = b.bindData(i, params.Value, "form", params.File); err != nil {
			return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
		}
	default:
		return ErrUnsupportedMediaType
	}
	return nil
}

// BindHeaders binds HTTP headers to a bindable object
func (b *DefaultBinder) BindHeaders(c Context, i interface{}) error {
	if err := b.bindData(i, c.Request().Header, "header", nil); err != nil {
		return NewHTTPError(http.StatusBadRequest, err.Error()).SetInternal(err)
	}
	return nil
}

// Bind implements the `Binder#Bind` function.
// Binding is done in following order: 1) path params; 2) query params; 3) request body. Each step COULD override previous
// step binded values. For single source binding use their own methods BindBody, BindQueryParams, BindPathParams.
func (b *DefaultBinder) Bind(i interface{}, c Context) (err error) {
	if err := b.BindPathParams(c, i); err != nil {
		return err
	}
	// Only bind query parameters for GET/DELETE/HEAD to avoid unexpected behavior with destination struct binding from body.
	// For example a request URL `&id=1&lang=en` with body `{"id":100,"lang":"de"}` would lead to precedence issues.
	// The HTTP method check restores pre-v4.1.11 behavior to avoid these problems (see issue #1670)
	method := c.Request().Method
	if method == http.MethodGet || method == http.MethodDelete || method == http.MethodHead {
		if err = b.BindQueryParams(c, i); err != nil {
			return err
		}
	}
	return b.BindBody(c, i)
}

var bindDataCoverage = make(map[int]bool)

const bindDataCoverageTotal = 61

// bindData will bind data ONLY fields in destination struct that have EXPLICIT tag
func (b *DefaultBinder) bindData(destination interface{}, data map[string][]string, tag string, dataFiles map[string][]*multipart.FileHeader) error {
	if destination == nil || (len(data) == 0 && len(dataFiles) == 0) {
		bindDataCoverage[0] = true
		return nil
	}
	bindDataCoverage[1] = true
	hasFiles := len(dataFiles) > 0
	typ := reflect.TypeOf(destination).Elem()
	val := reflect.ValueOf(destination).Elem()

	// Support binding to limited Map destinations:
	// - map[string][]string,
	// - map[string]string <-- (binds first value from data slice)
	// - map[string]interface{}
	// You are better off binding to struct but there are user who want this map feature. Source of data for these cases are:
	// params,query,header,form as these sources produce string values, most of the time slice of strings, actually.
	if typ.Kind() == reflect.Map && typ.Key().Kind() == reflect.String {
		bindDataCoverage[2] = true
		k := typ.Elem().Kind()
		isElemInterface := k == reflect.Interface
		isElemString := k == reflect.String
		isElemSliceOfStrings := k == reflect.Slice && typ.Elem().Elem().Kind() == reflect.String
		if !(isElemSliceOfStrings || isElemString || isElemInterface) {
			bindDataCoverage[3] = true
			return nil
		}
		bindDataCoverage[4] = true
		if val.IsNil() {
			bindDataCoverage[5] = true
			val.Set(reflect.MakeMap(typ))
		} else {
			bindDataCoverage[6] = true
		}
		for k, v := range data {
			bindDataCoverage[7] = true
			if isElemString {
				bindDataCoverage[8] = true
				val.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v[0]))
			} else if isElemInterface {
				// To maintain backward compatibility, we always bind to the first string value
				// and not the slice of strings when dealing with map[string]interface{}{}
				bindDataCoverage[9] = true
				val.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v[0]))
			} else {
				bindDataCoverage[10] = true
				val.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
			}
		}
		bindDataCoverage[11] = true
		return nil
	}
	bindDataCoverage[12] = true

	// !struct
	if typ.Kind() != reflect.Struct {
		bindDataCoverage[13] = true
		if tag == "param" || tag == "query" || tag == "header" {
			// incompatible type, data is probably to be found in the body
			bindDataCoverage[14] = true
			return nil
		}
		bindDataCoverage[15] = true
		return errors.New("binding element must be a struct")
	}
	bindDataCoverage[16] = true

	for i := 0; i < typ.NumField(); i++ { // iterate over all destination fields
		bindDataCoverage[17] = true
		typeField := typ.Field(i)
		structField := val.Field(i)
		if typeField.Anonymous {
			bindDataCoverage[18] = true
			if structField.Kind() == reflect.Ptr {
				bindDataCoverage[19] = true
				structField = structField.Elem()
			} else {
				bindDataCoverage[20] = true
			}
		} else {
			bindDataCoverage[21] = true
		}
		if !structField.CanSet() {
			bindDataCoverage[22] = true
			continue
		} else {
			bindDataCoverage[23] = true
		}
		structFieldKind := structField.Kind()
		inputFieldName := typeField.Tag.Get(tag)
		if typeField.Anonymous && structFieldKind == reflect.Struct && inputFieldName != "" {
			// if anonymous struct with query/param/form tags, report an error
			bindDataCoverage[24] = true
			return errors.New("query/param/form tags are not allowed with anonymous struct field")
		}
		bindDataCoverage[25] = true

		if inputFieldName == "" {
			// If tag is nil, we inspect if the field is a not BindUnmarshaler struct and try to bind data into it (might contain fields with tags).
			// structs that implement BindUnmarshaler are bound only when they have explicit tag
			bindDataCoverage[26] = true
			if _, ok := structField.Addr().Interface().(BindUnmarshaler); !ok && structFieldKind == reflect.Struct {
				bindDataCoverage[27] = true
				if err := b.bindData(structField.Addr().Interface(), data, tag, dataFiles); err != nil {
					return err
				}
			} else {
				bindDataCoverage[28] = true
			}
			// does not have explicit tag and is not an ordinary struct - so move to next field
			continue
		} else {
			bindDataCoverage[29] = true
		}

		if hasFiles {
			bindDataCoverage[30] = true
			if ok, err := isFieldMultipartFile(structField.Type()); err != nil {
				bindDataCoverage[31] = true
				return err
			} else if ok {
				bindDataCoverage[32] = true
				if ok := setMultipartFileHeaderTypes(structField, inputFieldName, dataFiles); ok {
					bindDataCoverage[33] = true
					continue
				} else {
					bindDataCoverage[34] = true
				}
			} else {
				bindDataCoverage[35] = true
			}
		} else {
			bindDataCoverage[36] = true
		}

		inputValue, exists := data[inputFieldName]
		if !exists {
			// Go json.Unmarshal supports case-insensitive binding.  However the
			// url params are bound case-sensitive which is inconsistent.  To
			// fix this we must check all of the map values in a
			// case-insensitive search.
			bindDataCoverage[37] = true
			for k, v := range data {
				bindDataCoverage[38] = true
				if strings.EqualFold(k, inputFieldName) {
					bindDataCoverage[39] = true
					inputValue = v
					exists = true
					break
				} else {
					bindDataCoverage[40] = true
				}
			}
		} else {
			bindDataCoverage[41] = true
		}

		if !exists {
			bindDataCoverage[42] = true
			continue
		} else {
			bindDataCoverage[43] = true
		}

		// NOTE: algorithm here is not particularly sophisticated. It probably does not work with absurd types like `**[]*int`
		// but it is smart enough to handle niche cases like `*int`,`*[]string`,`[]*int` .

		// try unmarshalling first, in case we're dealing with an alias to an array type
		if ok, err := unmarshalInputsToField(typeField.Type.Kind(), inputValue, structField); ok {
			bindDataCoverage[44] = true
			if err != nil {
				bindDataCoverage[45] = true
				return err
			}
			bindDataCoverage[46] = true
			continue
		} else {
			bindDataCoverage[47] = true
		}

		if ok, err := unmarshalInputToField(typeField.Type.Kind(), inputValue[0], structField); ok {
			bindDataCoverage[48] = true
			if err != nil {
				bindDataCoverage[49] = true
				return err
			}
			bindDataCoverage[50] = true
			continue
		} else {
			bindDataCoverage[51] = true
		}

		// we could be dealing with pointer to slice `*[]string` so dereference it. There are weird OpenAPI generators
		// that could create struct fields like that.
		if structFieldKind == reflect.Pointer {
			bindDataCoverage[52] = true
			structFieldKind = structField.Elem().Kind()
			structField = structField.Elem()
		} else {
			bindDataCoverage[53] = true
		}

		if structFieldKind == reflect.Slice {
			bindDataCoverage[54] = true
			sliceOf := structField.Type().Elem().Kind()
			numElems := len(inputValue)
			slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
			for j := 0; j < numElems; j++ {
				bindDataCoverage[55] = true
				if err := setWithProperType(sliceOf, inputValue[j], slice.Index(j)); err != nil {
					bindDataCoverage[56] = true
					return err
				}
				bindDataCoverage[57] = true
			}
			structField.Set(slice)
			continue
		} else {
			bindDataCoverage[58] = true
		}

		if err := setWithProperType(structFieldKind, inputValue[0], structField); err != nil {
			bindDataCoverage[59] = true
			return err
		}
		bindDataCoverage[60] = true
	}
	return nil
}

func setWithProperType(valueKind reflect.Kind, val string, structField reflect.Value) error {
	// But also call it here, in case we're dealing with an array of BindUnmarshalers
	if ok, err := unmarshalInputToField(valueKind, val, structField); ok {
		return err
	}

	switch valueKind {
	case reflect.Ptr:
		return setWithProperType(structField.Elem().Kind(), val, structField.Elem())
	case reflect.Int:
		return setIntField(val, 0, structField)
	case reflect.Int8:
		return setIntField(val, 8, structField)
	case reflect.Int16:
		return setIntField(val, 16, structField)
	case reflect.Int32:
		return setIntField(val, 32, structField)
	case reflect.Int64:
		return setIntField(val, 64, structField)
	case reflect.Uint:
		return setUintField(val, 0, structField)
	case reflect.Uint8:
		return setUintField(val, 8, structField)
	case reflect.Uint16:
		return setUintField(val, 16, structField)
	case reflect.Uint32:
		return setUintField(val, 32, structField)
	case reflect.Uint64:
		return setUintField(val, 64, structField)
	case reflect.Bool:
		return setBoolField(val, structField)
	case reflect.Float32:
		return setFloatField(val, 32, structField)
	case reflect.Float64:
		return setFloatField(val, 64, structField)
	case reflect.String:
		structField.SetString(val)
	default:
		return errors.New("unknown type")
	}
	return nil
}

func unmarshalInputsToField(valueKind reflect.Kind, values []string, field reflect.Value) (bool, error) {
	if valueKind == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	fieldIValue := field.Addr().Interface()
	unmarshaler, ok := fieldIValue.(bindMultipleUnmarshaler)
	if !ok {
		return false, nil
	}
	return true, unmarshaler.UnmarshalParams(values)
}

func unmarshalInputToField(valueKind reflect.Kind, val string, field reflect.Value) (bool, error) {
	if valueKind == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}

	fieldIValue := field.Addr().Interface()
	switch unmarshaler := fieldIValue.(type) {
	case BindUnmarshaler:
		return true, unmarshaler.UnmarshalParam(val)
	case encoding.TextUnmarshaler:
		return true, unmarshaler.UnmarshalText([]byte(val))
	}

	return false, nil
}

func setIntField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intVal, err := strconv.ParseInt(value, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

func setUintField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintVal, err := strconv.ParseUint(value, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

func setBoolField(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolVal, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

func setFloatField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatVal, err := strconv.ParseFloat(value, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}

var (
	// NOT supported by bind as you can NOT check easily empty struct being actual file or not
	multipartFileHeaderType = reflect.TypeOf(multipart.FileHeader{})
	// supported by bind as you can check by nil value if file existed or not
	multipartFileHeaderPointerType      = reflect.TypeOf(&multipart.FileHeader{})
	multipartFileHeaderSliceType        = reflect.TypeOf([]multipart.FileHeader(nil))
	multipartFileHeaderPointerSliceType = reflect.TypeOf([]*multipart.FileHeader(nil))
)

func isFieldMultipartFile(field reflect.Type) (bool, error) {
	switch field {
	case multipartFileHeaderPointerType,
		multipartFileHeaderSliceType,
		multipartFileHeaderPointerSliceType:
		return true, nil
	case multipartFileHeaderType:
		return true, errors.New("binding to multipart.FileHeader struct is not supported, use pointer to struct")
	default:
		return false, nil
	}
}

func setMultipartFileHeaderTypes(structField reflect.Value, inputFieldName string, files map[string][]*multipart.FileHeader) bool {
	fileHeaders := files[inputFieldName]
	if len(fileHeaders) == 0 {
		return false
	}

	result := true
	switch structField.Type() {
	case multipartFileHeaderPointerSliceType:
		structField.Set(reflect.ValueOf(fileHeaders))
	case multipartFileHeaderSliceType:
		headers := make([]multipart.FileHeader, len(fileHeaders))
		for i, fileHeader := range fileHeaders {
			headers[i] = *fileHeader
		}
		structField.Set(reflect.ValueOf(headers))
	case multipartFileHeaderPointerType:
		structField.Set(reflect.ValueOf(fileHeaders[0]))
	default:
		result = false
	}

	return result
}
