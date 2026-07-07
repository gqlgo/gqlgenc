# client

## Remove MarshalJSON function
The client does not have an equivalent to client.MarshalJSON. Instead, use json.Marshal.
Therefore, enum types must have a json.Marshaler implementation.
In gqlgen v1.17.71 and later, json.Marshaler code is generated for enum types.
```go
func (e *EnumValue) UnmarshalJSON(b []byte) error {
       s, err := strconv.Unquote(string(b))
       if err != nil {
               return err
       }
       return e.UnmarshalGQL(s)
}

func (e EnumValue) MarshalJSON() ([]byte, error) {
       var buf bytes.Buffer
       e.MarshalGQL(&buf)
       return buf.Bytes(), nil
}
```

- Reference
  - https://github.com/99designs/gqlgen/pull/3663

## Support omitzero
We now support omitzero introduced in Go 1.24.
https://pkg.go.dev/github.com/go-json-experiment/json#example-package-OmitFields
To set omitzero in gqlgen, use the following configuration in `gqlgen.yaml`:
```yaml
enable_model_json_omitzero_tag: true
```

- Reference
 - https://github.com/99designs/gqlgen/pull/3659

## Now able to distinguish between null and undefined
Some APIs behave differently for null and undefined.
You can now distinguish between null and undefined as follows:

- json: `{}` means undefined, no effect to name
```go
input := UserUpdateInput{
		Name:         graphql.Omittable[string]{},
}
```

- json: `{"name":null}` means null, set empty string to name
```go
input := UserUpdateInput{
		Name:         graphql.OmittableOf[string](null),
}
```

The Omittable type is generated when `nullable_input_omittable: true` is set in `gqlgen.yaml`. To omit, please set `enable_model_json_omitzero_tag: true` to add the omitzero tag.
To omit when undefined, use gqlgen v1.17.71 or later. The IsZero method has been added to the Omittable type.

```yaml
nullable_input_omittable: true

enable_model_json_omitzero_tag: true
```

```go
type UserUpdateInput struct {
    Name graphql.Omittable[*string] `json:"name,omitzero"`
```

- Reference
  - https://github.com/99designs/gqlgen/pull/3660


## Support application/graphql-response+json;charset=utf-8
- Reference
  - https://graphql.org/learn/serving-over-http/