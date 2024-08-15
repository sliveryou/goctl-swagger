package generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/zeromicro/go-zero/tools/goctl/api/spec"
	"github.com/zeromicro/go-zero/tools/goctl/config"
	"github.com/zeromicro/go-zero/tools/goctl/plugin"
	"github.com/zeromicro/go-zero/tools/goctl/util/format"
)

var (
	strColon        = []byte(":")
	defaultResponse = parseDefaultResponse()
)

const (
	defaultOption   = "default"
	stringOption    = "string"
	optionalOption  = "optional"
	omitemptyOption = "omitempty"
	optionsOption   = "options"
	rangeOption     = "range"
	exampleOption   = "example"
	optionSeparator = "|"
	equalToken      = "="
	atRespDoc       = "@respdoc-"

	tagKeyHeader   = "header"
	tagKeyPath     = "path"
	tagKeyForm     = "form"
	tagKeyJson     = "json"
	tagKeyValidate = "validate"
	tagKeyExample  = "example"

	// DefaultResponseJson default response pack json structure.
	DefaultResponseJson = `[{"name":"trace_id","type":"string","description":"链路追踪id","example":"a1b2c3d4e5f6g7h8"},{"name":"code","type":"integer","description":"状态码","example":0},{"name":"msg","type":"string","description":"消息","example":"ok"},{"name":"data","type":"object","description":"数据","is_data":true}]`
)

func parseRangeOption(option string) (min, max float64, ok bool) {
	const str = "\\[([+-]?\\d+(\\.\\d+)?):([+-]?\\d+(\\.\\d+)?)\\]"
	result := regexp.MustCompile(str).FindStringSubmatch(option)
	if len(result) != 5 {
		return 0, 0, false
	}

	var err error
	min, err = strconv.ParseFloat(result[1], 64)
	if err != nil {
		return 0, 0, false
	}

	max, err = strconv.ParseFloat(result[3], 64)
	if err != nil {
		return 0, 0, false
	}

	if max < min {
		return min, min, true
	}
	return min, max, true
}

func applyGenerate(p *plugin.Plugin, host, basePath, schemes, pack, response string) (*swaggerObject, error) {
	title, _ := strconv.Unquote(p.Api.Info.Properties["title"])
	version, _ := strconv.Unquote(p.Api.Info.Properties["version"])
	desc, _ := strconv.Unquote(p.Api.Info.Properties["desc"])

	s := swaggerObject{
		Swagger:           "2.0",
		Schemes:           []string{"http", "https"},
		Consumes:          []string{"application/json"},
		Produces:          []string{"application/json"},
		Paths:             make(swaggerPathsObject),
		Definitions:       make(swaggerDefinitionsObject),
		StreamDefinitions: make(swaggerDefinitionsObject),
		Info: swaggerInfoObject{
			Title:       title,
			Version:     version,
			Description: desc,
		},
	}
	if len(host) > 0 {
		s.Host = host
	}
	if len(basePath) > 0 {
		s.BasePath = basePath
	}

	if len(schemes) > 0 {
		supportedSchemes := []string{"http", "https", "ws", "wss"}
		ss := strings.Split(schemes, ",")
		for i := range ss {
			scheme := ss[i]
			scheme = strings.TrimSpace(scheme)
			if !contains(supportedSchemes, scheme) {
				log.Fatalf("unsupport scheme: [%s], only support [http, https, ws, wss]", scheme)
			}
			ss[i] = scheme
		}
		s.Schemes = ss
	}
	s.SecurityDefinitions = swaggerSecurityDefinitionsObject{}
	newSecDefValue := swaggerSecuritySchemeObject{}
	newSecDefValue.Name = "Authorization"
	newSecDefValue.Description = "Enter JWT Bearer token **_only_**"
	newSecDefValue.Type = "apiKey"
	newSecDefValue.In = "header"
	s.SecurityDefinitions["apiKey"] = newSecDefValue

	// s.Security = append(s.Security, swaggerSecurityRequirementObject{"apiKey": []string{}})

	dataKey := "data"
	if pack != "" {
		resp := defaultResponse
		if response != "" {
			r, dk, err := parseResponse(response)
			if err != nil {
				return nil, err
			}
			resp = r
			dataKey = dk
		}
		s.Definitions[pack] = resp
	}

	requestResponseRefs := refMap{}
	renderServiceRoutes(p.Api.Service, p.Api.Service.Groups, s.Paths, requestResponseRefs, pack, dataKey)
	renderReplyAsDefinition(s.Definitions, p.Api.Types, requestResponseRefs)

	return &s, nil
}

func renderServiceRoutes(service spec.Service, groups []spec.Group, paths swaggerPathsObject, requestResponseRefs refMap, pack, dataKey string) {
	for _, group := range groups {
		for _, route := range group.Routes {
			var (
				pathParamMap             = make(map[string]swaggerParameterObject)
				method                   = strings.ToUpper(route.Method)
				parameters               swaggerParametersObject
				hasBody                  bool
				containForm, containJson bool
			)

			path := group.GetAnnotation("prefix") + route.Path
			if path[0] != '/' {
				path = "/" + path
			}

			if m := strings.ToUpper(route.Method); m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch || m == http.MethodDelete {
				hasBody = true
			}

			if countParams(path) > 0 {
				p := strings.Split(path, "/")
				for i := range p {
					part := p[i]
					if strings.Contains(part, ":") {
						key := strings.TrimPrefix(p[i], ":")
						path = strings.Replace(path, ":"+key, "{"+key+"}", 1)
						spo := swaggerParameterObject{
							Name:     key,
							In:       "path",
							Required: true,
							Type:     "string",
						}

						// extend the comment functionality
						// to allow query string parameters definitions
						// EXAMPLE:
						// @doc(
						// 	summary: "Get Cart"
						// 	description: "returns a shopping cart if one exists"
						// 	customerId: "customer id"
						// )
						//
						// the format for a parameter is
						// paramName: "the param description"
						//

						prop := route.AtDoc.Properties[key]
						if prop != "" {
							// remove quotes
							spo.Description = strings.Trim(prop, "\"")
						}

						pathParamMap[spo.Name] = spo
					}
				}
			}

			// parse "file_*" or "file_array_*" key from the @doc
			// "*" means the file field name, it's like this below:
			// 	@doc (
			//		file_upload: "false, 上传文件"
			//      file_array_upload: "false, 上传文件数组"
			//	)
			// its properties are separated by commas
			// first one represents the file is it required
			// second one represents the file description

			var keys []string
			for key := range route.AtDoc.Properties {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := route.AtDoc.Properties[k]
				if strings.HasPrefix(k, "file_") {
					name := strings.TrimPrefix(k, "file_")
					if strings.HasPrefix(k, "file_array_") {
						name = strings.TrimPrefix(k, "file_array_") + "[]"
					}
					spo := swaggerParameterObject{
						Name: name,
						In:   "formData",
						Type: "file",
					}
					if properties := strings.Split(strings.Trim(v, `"`), ","); len(properties) > 0 {
						isRequired, _ := strconv.ParseBool(strings.TrimSpace(properties[0]))
						spo.Required = isRequired
						if len(properties) > 1 {
							spo.Description = strings.TrimSpace(properties[1])
						}
					}
					parameters = append(parameters, spo)
					containForm = true
				}
			}

			if defineStruct, ok := route.RequestType.(spec.DefineStruct); ok {
				for _, member := range defineStruct.Members {
					f, j := renderMember(pathParamMap, &parameters, member, method)
					if f {
						containForm = true
					}
					if j {
						containJson = true
					}
				}

				if len(pathParamMap) > 0 {
					for _, p := range pathParamMap {
						parameters = append(parameters, p)
					}
				}
				if hasBody && containJson {
					reqRef := "#/definitions/" + route.RequestType.Name()

					if len(route.RequestType.Name()) > 0 {
						schema := swaggerSchemaObject{
							schemaCore: schemaCore{
								Ref: reqRef,
							},
						}

						parameter := swaggerParameterObject{
							Name:     "body",
							In:       "body",
							Required: true,
							Schema:   &schema,
						}

						doc := strings.Join(route.RequestType.Documents(), ",")
						doc = strings.TrimSpace(strings.ReplaceAll(doc, "//", ""))

						if doc != "" {
							parameter.Description = doc
						}

						parameters = append(parameters, parameter)
					}
				}
			}

			pathItemObject, ok := paths[path]
			if !ok {
				pathItemObject = swaggerPathItemObject{}
			}

			desc := "A successful response."
			respSchema := schemaCore{}
			// respRef := swaggerSchemaObject{}
			if route.ResponseType != nil && len(route.ResponseType.Name()) > 0 {
				if strings.HasPrefix(route.ResponseType.Name(), "[]") {
					refTypeName := strings.Replace(route.ResponseType.Name(), "[", "", 1)
					refTypeName = strings.Replace(refTypeName, "]", "", 1)
					refTypeName = strings.TrimPrefix(refTypeName, "*") // remove array item pointer

					respSchema.Type = "array"
					respSchema.Items = &swaggerItemsObject{Ref: "#/definitions/" + refTypeName}
				} else {
					respSchema.Ref = "#/definitions/" + route.ResponseType.Name()
				}
			}
			tags := service.Name
			if value := group.GetAnnotation("group"); len(value) > 0 {
				namingFormat, err := format.FileNamingFormat(config.DefaultFormat, tags)
				if err != nil {
					return
				}

				tags = filepath.Join(namingFormat, value)
			}

			if value := group.GetAnnotation("swtags"); len(value) > 0 {
				namingFormat, err := format.FileNamingFormat(config.DefaultFormat, tags)
				if err != nil {
					return
				}

				tags = filepath.Join(namingFormat, value)
			}

			schema := swaggerSchemaObject{
				schemaCore: respSchema,
			}
			if pack != "" {
				schema = swaggerSchemaObject{
					AllOf: []swaggerSchemaObject{
						{schemaCore: schemaCore{Ref: "#/definitions/" + strings.TrimPrefix(pack, "/")}},
						{schemaCore: schemaCore{Type: "object"}, Properties: &swaggerSchemaObjectProperties{{Key: dataKey, Value: respSchema}}},
					},
				}
			}
			operationObject := &swaggerOperationObject{
				Tags:       []string{tags},
				Parameters: parameters,
				Responses: swaggerResponsesObject{
					"200": swaggerResponseObject{
						Description: desc,
						Schema:      schema,
					},
				},
			}

			// if request has body, there is no way to distinguish query param and form param.
			// because they both share the "form" tag, the same param will appear in both query and body.
			if hasBody && containForm && !containJson && method != http.MethodDelete {
				operationObject.Consumes = []string{"multipart/form-data", "application/x-www-form-urlencoded"}

				for i := range operationObject.Parameters {
					if operationObject.Parameters[i].In == "query" {
						operationObject.Parameters[i].In = "formData"
					}
				}
			}

			for _, v := range route.Doc {
				markerIndex := strings.Index(v, atRespDoc)
				if markerIndex >= 0 {
					l := strings.Index(v, "(")
					r := strings.Index(v, ")")
					code := strings.TrimSpace(v[markerIndex+len(atRespDoc) : l])
					var comment string
					commentIndex := strings.Index(v, "//")
					if commentIndex > 0 {
						comment = strings.TrimSpace(strings.Trim(v[commentIndex+2:], "*/"))
					}
					content := strings.TrimSpace(v[l+1 : r])
					if strings.Index(v, ":") > 0 {
						lines := strings.Split(content, "\n")
						kv := make(map[string]string, len(lines))
						for _, line := range lines {
							sep := strings.Index(line, ":")
							key := strings.TrimSpace(line[:sep])
							value := strings.TrimSpace(line[sep+1:])
							kv[key] = value
						}
						kvByte, err := json.Marshal(kv)
						if err != nil {
							continue
						}
						operationObject.Responses[code] = swaggerResponseObject{
							Description: comment,
							Schema: swaggerSchemaObject{
								schemaCore: schemaCore{
									Example: string(kvByte),
								},
							},
						}
					} else if len(content) > 0 {
						operationObject.Responses[code] = swaggerResponseObject{
							Description: comment,
							Schema: swaggerSchemaObject{
								schemaCore: schemaCore{
									Ref: "#/definitions/" + content,
								},
							},
						}
					}
				}
			}

			// set OperationID
			operationObject.OperationID = route.Handler

			for _, param := range operationObject.Parameters {
				if param.Schema != nil && param.Schema.Ref != "" {
					requestResponseRefs[param.Schema.Ref] = struct{}{}
				}
			}
			operationObject.Summary = strings.ReplaceAll(route.JoinedDoc(), "\"", "")

			if len(route.AtDoc.Properties) > 0 {
				operationObject.Description, _ = strconv.Unquote(route.AtDoc.Properties["description"])
			}

			operationObject.Description = strings.ReplaceAll(operationObject.Description, "\"", "")

			if group.GetAnnotation("jwt") != "" ||
				strings.Contains(strings.ToLower(group.GetAnnotation("middleware")), "jwt") {
				operationObject.Security = &[]swaggerSecurityRequirementObject{{"apiKey": []string{}}}
			}

			switch method {
			case http.MethodGet:
				pathItemObject.Get = operationObject
			case http.MethodPost:
				pathItemObject.Post = operationObject
			case http.MethodDelete:
				pathItemObject.Delete = operationObject
			case http.MethodPut:
				pathItemObject.Put = operationObject
			case http.MethodPatch:
				pathItemObject.Patch = operationObject
			}

			paths[path] = pathItemObject
		}
	}
}

// renderMember collect param property from spec.Member, return whether there exists form fields and json fields.
func renderMember(pathParamMap map[string]swaggerParameterObject,
	parameters *swaggerParametersObject, member spec.Member, method string,
) (containForm, containJson bool) {
	if embedStruct, isEmbed := member.Type.(spec.DefineStruct); isEmbed {
		for _, m := range embedStruct.Members {
			f, j := renderMember(pathParamMap, parameters, m, method)
			if f {
				containForm = true
			}
			if j {
				containJson = true
			}
		}
		return containForm, containJson
	}

	p := renderStruct(member)

	if p.In == "" {
		if method == http.MethodGet {
			p.In = "query"
		} else {
			p.In = "body"
		}
	}
	if p.In == "body" {
		containJson = true
		return containForm, containJson
	}
	if p.In == "query" {
		containForm = true
	}

	// overwrite path parameter if we get a user defined one from struct.
	if op, ok := pathParamMap[p.Name]; p.In == "path" && ok {
		if p.Description == "" && op.Description != "" {
			p.Description = op.Description
		}
		delete(pathParamMap, p.Name)
	}

	*parameters = append(*parameters, p)

	return containForm, containJson
}

func fillValidateOption(s *swaggerSchemaObject, opt string) {
	kv := strings.SplitN(opt, "=", 2)
	if len(kv) != 2 {
		return
	}
	switch kv[0] {
	case "oneof":
		var es []string
		// oneof='red green' 'blue yellow'
		if strings.Contains(kv[1], "'") {
			es = strings.Split(kv[1], "' '")
			es[0] = strings.TrimPrefix(es[0], "'")
			es[len(es)-1] = strings.TrimSuffix(es[len(es)-1], "'")
		} else {
			es = strings.Split(kv[1], " ")
		}
		s.Enum = es
	case "min", "gte", "gt":
		switch s.Type {
		case "number", "integer":
			s.Minimum, _ = strconv.ParseFloat(kv[1], 64)
		case "array", "string":
			v, err := strconv.ParseUint(kv[1], 10, 64)
			if err != nil {
				break
			}
			if s.Type == "array" {
				s.MinItems = v
			} else {
				s.MinLength = v
			}
		}
		if kv[0] == "gt" {
			s.ExclusiveMinimum = true
		}
	case "max", "lte", "lt":
		switch s.Type {
		case "number", "integer":
			s.Maximum, _ = strconv.ParseFloat(kv[1], 64)
		case "array", "string":
			v, err := strconv.ParseUint(kv[1], 10, 64)
			if err != nil {
				break
			}
			if s.Type == "array" {
				s.MaxItems = v
			} else {
				s.MaxLength = v
			}
		}
		if kv[0] == "lt" {
			s.ExclusiveMaximum = true
		}
	}
}

func fillValidate(s *swaggerSchemaObject, tag *spec.Tag) {
	if tag.Key != tagKeyValidate {
		return
	}
	fillValidateOption(s, tag.Name)
	for _, opt := range tag.Options {
		fillValidateOption(s, opt)
	}
}

func fillExample(s *swaggerSchemaObject, tag *spec.Tag) {
	if tag.Key != tagKeyExample {
		return
	}
	switch s.Type {
	case "string":
		s.Example = tag.Name
	case "integer":
		s.Example, _ = strconv.Atoi(tag.Name)
	case "number":
		s.Example, _ = strconv.ParseFloat(tag.Name, 64)
	case "boolean":
		s.Example, _ = strconv.ParseBool(tag.Name)
	case "array":
		s.Example = append([]string{tag.Name}, tag.Options...)
	}
}

// renderStruct only need to deal with params in header/path/query
func renderStruct(member spec.Member) swaggerParameterObject {
	tempKind := swaggerMapTypes[strings.ReplaceAll(member.Type.Name(), "[]", "")]

	ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())
	if !ok {
		ftype = tempKind.String()
		format = "UNKNOWN"
	}
	sp := swaggerParameterObject{In: "", Type: ftype, Format: format, Schema: new(swaggerSchemaObject)}

	for _, tag := range member.Tags() {
		switch tag.Key {
		case tagKeyHeader:
			sp.In = "header"
		case tagKeyPath:
			sp.In = "path"
		case tagKeyForm:
			sp.In = "query"
		case tagKeyJson:
			sp.In = "body"
		case tagKeyValidate:
			fillValidate(sp.Schema, tag)
			sp.Enum = sp.Schema.Enum
			continue
		default:
			continue
		}

		sp.Name = tag.Name
		if len(tag.Options) == 0 {
			sp.Required = true
			continue
		}

		required := true
		for _, option := range tag.Options {
			if strings.HasPrefix(option, optionsOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					sp.Enum = strings.Split(segs[1], optionSeparator)
				}
			}

			if strings.HasPrefix(option, rangeOption) {
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						sp.Schema.Minimum = min
						sp.Schema.Maximum = max
					}
				}
			}

			if strings.HasPrefix(option, defaultOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Default = segs[1]
				}
			} else if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
				required = false
			}

			if strings.HasPrefix(option, exampleOption) {
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					sp.Example = segs[1]
				}
			}
		}
		sp.Required = required
	}

	if sp.Name == "" {
		sp.Name = member.Name
	}

	if len(member.Comment) > 0 {
		sp.Description = strings.TrimSpace(strings.ReplaceAll(strings.TrimLeft(member.Comment, "/"), "\\n", "\n"))
	}

	// schema is defined when "in" == "body"
	if sp.In != "body" {
		sp.Copy(sp.Schema)
		sp.Schema = nil
	}

	return sp
}

func renderReplyAsDefinition(d swaggerDefinitionsObject, p []spec.Type, _ refMap) {
	// record inline struct
	inlineMap := make(map[string][]string)
	for _, i2 := range p {
		var formFields, untaggedFields swaggerSchemaObjectProperties

		schema := swaggerSchemaObject{
			schemaCore: schemaCore{
				Type: "object",
			},
			Properties: new(swaggerSchemaObjectProperties),
		}
		defineStruct, _ := i2.(spec.DefineStruct)

		schema.Title = defineStruct.Name()

		for _, member := range defineStruct.Members {
			inlines := collectProperties(schema.Properties, &formFields, &untaggedFields, member)
			if len(inlines) > 0 {
				inlineMap[defineStruct.Name()] = inlines
			}
			for _, tag := range member.Tags() {
				if tag.Key != tagKeyForm && tag.Key != tagKeyJson {
					continue
				}
				if len(tag.Options) == 0 {
					if !contains(schema.Required, tag.Name) && tag.Name != "required" {
						schema.Required = append(schema.Required, tag.Name)
					}
					continue
				}

				required := true
				for _, option := range tag.Options {
					// case strings.HasPrefix(option, defaultOption):
					// case strings.HasPrefix(option, optionsOption):

					if strings.HasPrefix(option, optionalOption) || strings.HasPrefix(option, omitemptyOption) {
						required = false
					}
				}

				if required && !contains(schema.Required, tag.Name) {
					schema.Required = append(schema.Required, tag.Name)
				}
			}
		}
		// if there exists any json fields, form fields are ignored (considered to be params in query).
		if len(*schema.Properties) == 0 && len(formFields) > 0 {
			*schema.Properties = formFields
		}
		if len(untaggedFields) > 0 {
			*schema.Properties = append(*schema.Properties, untaggedFields...)
		}

		d[i2.Name()] = schema
	}

	// inherit properties
	for name, inlines := range inlineMap {
		if baseStruct, ok := d[name]; ok {
			tmp := new(swaggerSchemaObjectProperties)
			for _, inlineName := range inlines {
				if inlineStruct, ok := d[inlineName]; ok {
					*tmp = append(*tmp, *inlineStruct.Properties...)
				}
			}
			// append from the head
			if len(*tmp) > 0 {
				*baseStruct.Properties = append(*tmp, *baseStruct.Properties...)
			}
		}
	}
}

func collectProperties(jsonFields, formFields, untaggedFields *swaggerSchemaObjectProperties, member spec.Member) (inlines []string) {
	in := fieldIn(member)
	if in == tagKeyHeader || in == tagKeyPath {
		return inlines
	}

	name := member.Name
	if tag, err := member.GetPropertyName(); err == nil {
		name = tag
	}
	if name == "" {
		memberStruct, _ := member.Type.(spec.DefineStruct)
		// currently go-zero does not support show members of nested struct over 2 levels(include).
		// but openapi 2.0 does not support inline schema, we have no choice but use an empty properties name
		// which is not friendly to the user.
		if len(memberStruct.Members) > 0 {
			for _, m := range memberStruct.Members {
				is := collectProperties(jsonFields, formFields, untaggedFields, m)
				inlines = append(inlines, is...)
			}
			return inlines
		}
		inlines = append(inlines, memberStruct.Name())
		return inlines
	}

	kv := keyVal{Key: name, Value: schemaOfField(member)}
	switch in {
	case tagKeyJson:
		*jsonFields = append(*jsonFields, kv)
	case tagKeyForm:
		*formFields = append(*formFields, kv)
	default:
		*untaggedFields = append(*untaggedFields, kv)
	}

	return inlines
}

func fieldIn(member spec.Member) string {
	for _, tag := range member.Tags() {
		if tag.Key == tagKeyPath || tag.Key == tagKeyHeader || tag.Key == tagKeyForm || tag.Key == tagKeyJson {
			return tag.Key
		}
	}

	return ""
}

func schemaOfField(member spec.Member) swaggerSchemaObject {
	ret := swaggerSchemaObject{}

	var core schemaCore

	kind := swaggerMapTypes[member.Type.Name()]
	var props *swaggerSchemaObjectProperties

	comment := member.GetComment()
	comment = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(comment, "//", ""), "\\n", "\n"))

	switch ft := kind; ft {
	case reflect.Invalid: // []Struct 也有可能是 Struct
		// []Struct
		// map[ArrayType:map[Star:map[StringExpr:UserSearchReq] StringExpr:*UserSearchReq] StringExpr:[]*UserSearchReq]
		refTypeName := strings.Replace(member.Type.Name(), "[", "", 1)
		refTypeName = strings.Replace(refTypeName, "]", "", 1)
		refTypeName = strings.Replace(refTypeName, "*", "", 1)
		refTypeName = strings.Replace(refTypeName, "{", "", 1)
		refTypeName = strings.Replace(refTypeName, "}", "", 1)
		// interface

		if refTypeName == "interface" {
			core = schemaCore{Type: "object"}
		} else if refTypeName == "mapstringstring" {
			core = schemaCore{Type: "object"}
		} else if strings.HasPrefix(refTypeName, "[]") {
			core = schemaCore{Type: "array"}

			tempKind := swaggerMapTypes[strings.ReplaceAll(refTypeName, "[]", "")]
			ftype, format, ok := primitiveSchema(tempKind, refTypeName)
			if ok {
				core.Items = &swaggerItemsObject{Type: ftype, Format: format}
			} else {
				core.Items = &swaggerItemsObject{Type: ft.String(), Format: "UNKNOWN"}
			}
		} else {
			core = schemaCore{
				Ref: "#/definitions/" + refTypeName,
			}
		}
	case reflect.Slice:
		tempKind := swaggerMapTypes[strings.ReplaceAll(member.Type.Name(), "[]", "")]
		ftype, format, ok := primitiveSchema(tempKind, member.Type.Name())

		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	default:
		ftype, format, ok := primitiveSchema(ft, member.Type.Name())
		if ok {
			core = schemaCore{Type: ftype, Format: format}
		} else {
			core = schemaCore{Type: ft.String(), Format: "UNKNOWN"}
		}
	}

	switch ft := kind; ft {
	case reflect.Slice:
		ret = swaggerSchemaObject{
			schemaCore: schemaCore{
				Type:  "array",
				Items: (*swaggerItemsObject)(&core),
			},
		}
	case reflect.Invalid:
		// 判断是否数组
		if strings.HasPrefix(member.Type.Name(), "[]") {
			ret = swaggerSchemaObject{
				schemaCore: schemaCore{
					Type:  "array",
					Items: (*swaggerItemsObject)(&core),
				},
			}
		} else {
			ret = swaggerSchemaObject{
				schemaCore: core,
				Properties: props,
			}
		}
	default:
		ret = swaggerSchemaObject{
			schemaCore: core,
			Properties: props,
		}
	}
	ret.Description = comment

	for _, tag := range member.Tags() {
		if tag.Key == tagKeyValidate {
			fillValidate(&ret, tag)
			continue
		} else if tag.Key == tagKeyExample {
			fillExample(&ret, tag)
			continue
		}
		if len(tag.Options) == 0 || tag.Key != tagKeyForm && tag.Key != tagKeyJson {
			continue
		}

		for _, option := range tag.Options {
			switch {
			case strings.HasPrefix(option, defaultOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Default = segs[1]
				}
			case strings.HasPrefix(option, optionsOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					ret.Enum = strings.Split(segs[1], optionSeparator)
				}
			case strings.HasPrefix(option, rangeOption):
				segs := strings.SplitN(option, equalToken, 2)
				if len(segs) == 2 {
					min, max, ok := parseRangeOption(segs[1])
					if ok {
						ret.Minimum = min
						ret.Maximum = max
					}
				}
			case strings.HasPrefix(option, exampleOption):
				segs := strings.Split(option, equalToken)
				if len(segs) == 2 {
					ret.Example = segs[1]
				}
			}
		}
	}

	return ret
}

// https://swagger.io/specification/ Data Types
func primitiveSchema(kind reflect.Kind, t string) (ftype, format string, ok bool) {
	switch kind {
	case reflect.Int:
		return "integer", "int32", true
	case reflect.Uint:
		return "integer", "uint32", true
	case reflect.Int8:
		return "integer", "int8", true
	case reflect.Uint8:
		return "integer", "uint8", true
	case reflect.Int16:
		return "integer", "int16", true
	case reflect.Uint16:
		return "integer", "uin16", true
	case reflect.Int64:
		return "integer", "int64", true
	case reflect.Uint64:
		return "integer", "uint64", true
	case reflect.Bool:
		return "boolean", "boolean", true
	case reflect.String:
		return "string", "", true
	case reflect.Float32:
		return "number", "float", true
	case reflect.Float64:
		return "number", "double", true
	case reflect.Slice:
		return strings.ReplaceAll(t, "[]", ""), "", true
	default:
		return "", "", false
	}
}

// StringToBytes converts string to byte slice without a memory allocation.
func stringToBytes(s string) (b []byte) {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}

func countParams(path string) uint16 {
	var n uint16
	s := stringToBytes(path)
	n += uint16(bytes.Count(s, strColon))
	return n
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func parseResponse(resp string) (swaggerSchemaObject, string, error) {
	var fields []responseField
	err := json.Unmarshal([]byte(resp), &fields)
	if err != nil {
		return swaggerSchemaObject{}, "", err
	}

	hasData := false
	dataKey := ""
	for _, field := range fields {
		if field.Name == "" || field.Type == "" {
			return swaggerSchemaObject{}, "", errors.New("响应字段参数错误")
		}
		if field.IsData {
			hasData = true
			dataKey = field.Name
		}
	}
	if !hasData {
		return swaggerSchemaObject{}, "", errors.New("请指定包装的数据字段")
	}

	properties := new(swaggerSchemaObjectProperties)
	response := swaggerSchemaObject{schemaCore: schemaCore{Type: "object"}}
	for _, field := range fields {
		*properties = append(*properties,
			keyVal{
				Key: field.Name,
				Value: swaggerSchemaObject{
					schemaCore:  schemaCore{Type: field.Type},
					Description: field.Description,
					Example:     field.Example,
				},
			})
	}
	response.Properties = properties

	return response, dataKey, nil
}

func parseDefaultResponse() swaggerSchemaObject {
	response, _, _ := parseResponse(DefaultResponseJson)
	return response
}
